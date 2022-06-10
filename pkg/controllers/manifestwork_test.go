package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"testing"

	hyp "github.com/openshift/hypershift/api/v1alpha1"

	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/meta"
	condmeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kindAndKey struct {
	schema.GroupVersionKind
	types.NamespacedName
}

func getHDforManifestWork() *hyd.HypershiftDeployment {
	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1", false)

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	testHD.Spec.InfraID = infraOut.InfraID
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)
	return testHD
}

func getClusterSetBinding(namespace string) *clusterv1beta1.ManagedClusterSetBinding {
	return &clusterv1beta1.ManagedClusterSetBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: namespace,
		},
		Spec: clusterv1beta1.ManagedClusterSetBindingSpec{
			ClusterSet: "dev",
		},
	}
}

func getClusterSet() *clusterv1beta1.ManagedClusterSet {
	return &clusterv1beta1.ManagedClusterSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dev",
		},
		Spec: clusterv1beta1.ManagedClusterSetSpec{},
	}
}

func getCluster(name string) *clusterv1.ManagedCluster {
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"vendor": "openshift",
			},
		},
		Spec: clusterv1.ManagedClusterSpec{},
	}
}

type manifestworkChecker struct {
	clt      client.Client
	ctx      context.Context
	key      types.NamespacedName
	resource map[kindAndKey]bool
	obj      *workv1.ManifestWork
	status   workv1.ManifestWorkStatus
	spec     workv1.ManifestWorkSpec
}

func newManifestResourceChecker(ctx context.Context, clt client.Client, key types.NamespacedName) (*manifestworkChecker, error) {
	m := &manifestworkChecker{ctx: ctx, clt: clt, key: key}
	err := m.update()
	return m, err
}

func (m *manifestworkChecker) update() error {
	manifestWork := &workv1.ManifestWork{}
	if err := m.clt.Get(m.ctx, m.key, manifestWork); err != nil {
		return err
	}

	wl := manifestWork.Spec.Workload.Manifests

	got := map[kindAndKey]bool{}

	for _, w := range wl {
		u := &unstructured.Unstructured{}
		if err := json.Unmarshal(w.Raw, u); err != nil {
			return fmt.Errorf("failed convert manifest to unstructured, err: %w", err)
		}

		k := kindAndKey{
			GroupVersionKind: u.GetObjectKind().GroupVersionKind(),
			NamespacedName:   genKeyFromObject(u),
		}

		got[k] = true
	}

	m.resource = got
	m.status = manifestWork.Status
	m.spec = manifestWork.Spec
	m.obj = manifestWork

	return nil
}

func (m *manifestworkChecker) shouldHave(res map[kindAndKey]bool) error {
	for k, shouldExist := range res {
		if shouldExist {
			if !m.resource[k] {
				return fmt.Errorf("%v should exist in manifestwork", k)
			}

			continue
		}

		if m.resource[k] {
			return fmt.Errorf("%v shouldn't exist in manifestwork", k)
		}
	}

	return nil
}

func (m *manifestworkChecker) shouldNotHave(res map[kindAndKey]bool) error {
	for k, _ := range res {
		if _, ok := m.resource[k]; ok {
			return fmt.Errorf("%v shouldn't exist in manifestwork", k)
		}
	}

	return nil
}

func getHostedClusterForManifestworkTest(testHD *hyd.HypershiftDeployment) *hyp.HostedCluster {
	hostedCluster := getHostedCluster(testHD)
	ap := &hyp.AWSPlatformSpec{
		Region:                    testHD.Spec.Infrastructure.Platform.AWS.Region,
		ControlPlaneOperatorCreds: corev1.LocalObjectReference{Name: testHD.Name + "-cpo-creds"},
		KubeCloudControllerCreds:  corev1.LocalObjectReference{Name: testHD.Name + "-cloud-ctrl-creds"},
		NodePoolManagementCreds:   corev1.LocalObjectReference{Name: testHD.Name + "-node-mgmt-creds"},
		EndpointAccess:            hyp.Public,
		ResourceTags: []hyp.AWSResourceTag{
			//set the resource tags to prevent the work always updating the hostedcluster resource on the hosting cluster.
			{
				Key:   "kubernetes.io/cluster/" + testHD.Spec.HostedClusterSpec.InfraID,
				Value: "owned",
			},
		},
	}
	hostedCluster.Spec.Platform.AWS = ap
	hostedCluster.Spec.PullSecret.Name = "test1-pull-secret"
	return hostedCluster
}

// TestManifestWorkFlow tests if the manifestwork is created
// and reference secret is put into manifestwork payload
func TestManifestWorkFlowBaseCase(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	client.Create(ctx, getPullSecret(testHD))

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	requiredResource := map[kindAndKey]bool{
		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Namespace"},
			NamespacedName: types.NamespacedName{
				Name: "default", Namespace: ""}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "HostedCluster"},
			NamespacedName: genKeyFromObject(testHD)}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "NodePool"},
			NamespacedName: genKeyFromObject(testHD)}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: helper.GetHostingNamespace(testHD)}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-cpo-creds", Namespace: helper.GetHostingNamespace(testHD)}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: helper.GetHostingNamespace(testHD)}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-pull-secret", Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err := newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldHave(requiredResource), "err nil when all requrie resource exist in manifestwork")
}

func TestManifestWorkFlowBaseCaseWithObjectRef(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		client.Create(ctx, np)
		defer client.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	client.Create(ctx, getPullSecret(testHD))
	client.Create(ctx, getAwsCpoSecret(testHD))
	client.Create(ctx, getAwsCloudCtrlSecret(testHD))
	client.Create(ctx, getAwsNodeMgmtSecret(testHD))

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	requiredResource := map[kindAndKey]bool{
		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Namespace"},
			NamespacedName: types.NamespacedName{
				Name: "default", Namespace: ""}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-cpo-creds", Namespace: helper.GetHostingNamespace(testHD)}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: helper.GetHostingNamespace(testHD)}}: true,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-pull-secret", Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err := newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldHave(requiredResource), "err nil when all requrie resource exist in manifestwork")
}

// TestManifestWorkFlowWithExtraConfigurations tests if the manifestwork is created
// and extra secret/configmap is put into manifestwork payload in addition to
// the required resource of TestManifestWorkFlow
func TestManifestWorkFlowWithExtraConfigurations(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	cfgSecretName := "hostedcluster-config-secret-1"
	cfgConfigName := "hostedcluster-config-configmap-1"
	cfgItemSecretName := "hostedcluster-config-item-1"
	cfgAdditionalTrustBundle := "hostedcluster-additionaltrustbundle"
	serviceAccountSigningKey := "hostedcluster-sask"

	insertConfigSecretAndConfigMap := func() {
		testHD.Spec.HostedClusterSpec.Configuration = &hyp.ClusterConfiguration{}
		testHD.Spec.HostedClusterSpec.Configuration.SecretRefs = []corev1.LocalObjectReference{
			corev1.LocalObjectReference{Name: cfgSecretName}}

		testHD.Spec.HostedClusterSpec.Configuration.ConfigMapRefs = []corev1.LocalObjectReference{
			corev1.LocalObjectReference{Name: cfgConfigName}}

		testHD.Spec.HostedClusterSpec.AdditionalTrustBundle = &corev1.LocalObjectReference{
			Name: cfgAdditionalTrustBundle,
		}

		testHD.Spec.HostedClusterSpec.ServiceAccountSigningKey = &corev1.LocalObjectReference{
			Name: serviceAccountSigningKey,
		}

		testHD.Spec.HostedClusterSpec.Configuration.Items = []runtime.RawExtension{runtime.RawExtension{Object: &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfgItemSecretName,
				Namespace: testHD.GetNamespace(),
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte(`docker-pull-secret`),
			},
		},
		}}
	}

	insertConfigSecretAndConfigMap()

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)

	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	se := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgSecretName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}

	client.Create(ctx, se)
	defer client.Delete(ctx, se)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgConfigName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string]string{
			".dockerconfigjson": "docker-configmap",
		},
	}

	cb := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgAdditionalTrustBundle,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string]string{
			"certs": "something special ABC XYZ",
		},
	}

	sask := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountSigningKey,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			"signingKey": []byte(`something special ABC XYZ`),
		},
	}

	client.Create(ctx, cm)
	defer client.Delete(ctx, cm)

	client.Create(ctx, cb)
	defer client.Delete(ctx, cb)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	client.Create(ctx, sask)
	defer client.Delete(ctx, sask)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgSecretName, Namespace: helper.GetHostingNamespace(testHD)}}: true,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgConfigName, Namespace: helper.GetHostingNamespace(testHD)}}: true,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgAdditionalTrustBundle, Namespace: helper.GetHostingNamespace(testHD)}}: true,
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: serviceAccountSigningKey, Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err := newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldHave(requiredResource), "err nil when all requrie resource exist in manifestwork")

}

func TestManifestWorkFlowWithExtraConfigurationsAndObjectRef(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	cfgSecretName := "hostedcluster-config-secret-1"
	cfgConfigName := "hostedcluster-config-configmap-1"
	cfgItemSecretName := "hostedcluster-config-item-1"
	cfgAdditionalTrustBundle := "hostedcluster-additionaltrustbundle"
	serviceAccountSigningKey := "hostedcluster-sask"

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	insertConfigSecretAndConfigMap := func() {
		hostedCluster.Spec.Configuration = &hyp.ClusterConfiguration{}
		hostedCluster.Spec.Configuration.SecretRefs = []corev1.LocalObjectReference{
			{Name: cfgSecretName}}

		hostedCluster.Spec.Configuration.ConfigMapRefs = []corev1.LocalObjectReference{
			{Name: cfgConfigName}}

		hostedCluster.Spec.AdditionalTrustBundle = &corev1.LocalObjectReference{
			Name: cfgAdditionalTrustBundle,
		}

		hostedCluster.Spec.ServiceAccountSigningKey = &corev1.LocalObjectReference{
			Name: serviceAccountSigningKey,
		}

		hostedCluster.Spec.Configuration.Items = []runtime.RawExtension{runtime.RawExtension{Object: &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cfgItemSecretName,
				Namespace: testHD.GetNamespace(),
			},
			Data: map[string][]byte{
				".dockerconfigjson": []byte(`docker-pull-secret`),
			},
		},
		}}
	}
	insertConfigSecretAndConfigMap()
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		client.Create(ctx, np)
		defer client.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	cpoSecret := getAwsCpoSecret(testHD)
	client.Create(ctx, cpoSecret)
	defer client.Delete(ctx, cpoSecret)

	cloudSecret := getAwsCloudCtrlSecret(testHD)
	client.Create(ctx, cloudSecret)
	defer client.Delete(ctx, cloudSecret)

	nodeSecret := getAwsNodeMgmtSecret(testHD)
	client.Create(ctx, nodeSecret)
	defer client.Delete(ctx, nodeSecret)

	se := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgSecretName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}

	client.Create(ctx, se)
	defer client.Delete(ctx, se)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgConfigName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string]string{
			".dockerconfigjson": "docker-configmap",
		},
	}

	cb := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfgAdditionalTrustBundle,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string]string{
			"certs": "something special ABC XYZ",
		},
	}

	sask := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccountSigningKey,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			"signingKey": []byte(`something special ABC XYZ`),
		},
	}

	client.Create(ctx, cm)
	defer client.Delete(ctx, cm)

	client.Create(ctx, cb)
	defer client.Delete(ctx, cb)

	testHD.Spec.HostedClusterRef = corev1.LocalObjectReference{Name: hostedCluster.Name}
	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	client.Create(ctx, sask)
	defer client.Delete(ctx, sask)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgSecretName, Namespace: helper.GetHostingNamespace(testHD)}}: true,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgConfigName, Namespace: helper.GetHostingNamespace(testHD)}}: true,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgAdditionalTrustBundle, Namespace: helper.GetHostingNamespace(testHD)}}: true,
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: serviceAccountSigningKey, Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err := newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldHave(requiredResource), "err nil when all requrie resource exist in manifestwork")

}

func TestManifestWorkFlowNoHostingCluster(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	client.Create(ctx, getPullSecret(testHD))

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	t.Log("Condition msg: " + c.Message)
	assert.Equal(t, constant.HostingClusterMissing, c.Message, "is equal when hostingCluster is missing")
}

// TestManifestWorkFlow tests if the manifestwork is created
// and referenece secret is put into manifestwork payload
func TestManifestWorkFlowWithSSHKey(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"
	testHD.Spec.Infrastructure.CloudProvider.Name = "aws"

	sshKeySecretName := fmt.Sprintf("%s-ssh-key", testHD.GetName())
	pullSecretName := fmt.Sprintf("%s-pull-secret", testHD.GetName())

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	hostedCluster.Spec.SSHKey.Name = sshKeySecretName
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		client.Create(ctx, np)
		defer client.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(context.Background(), testHD)

	_, err := hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.NotNil(t, err, "fail on missing pull secret")

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pullSecretName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	cpoSecret := getAwsCpoSecret(testHD)
	client.Create(ctx, cpoSecret)
	defer client.Delete(ctx, cpoSecret)

	cloudSecret := getAwsCloudCtrlSecret(testHD)
	client.Create(ctx, cloudSecret)
	defer client.Delete(ctx, cloudSecret)

	nodeSecret := getAwsNodeMgmtSecret(testHD)
	client.Create(ctx, nodeSecret)
	defer client.Delete(ctx, nodeSecret)

	sshKeySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sshKeySecretName,
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`ssh-key-secret`),
		},
	}

	err = client.Create(ctx, sshKeySecret)
	assert.Nil(t, err, "err nil when creating ssh key secret")
	defer client.Delete(ctx, sshKeySecret)

	_, err = hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successful")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: pullSecretName, Namespace: helper.GetHostingNamespace(testHD)}}: true,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: sshKeySecretName, Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err := newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldHave(requiredResource), "err nil when all requrie resource exist in manifestwork")

	t.Log("test hypershiftDeployment remove ssh key reference")

	var fakeObjList2 []runtime.Object
	hostedCluster.Spec.SSHKey.Name = ""
	fakeObjList2 = append(fakeObjList2, hostedCluster)
	fakeObjList2 = append(fakeObjList2, fakeObjList[1:]...)
	initFakeClient(hdr, fakeObjList2...)

	_, err = hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successful")

	deleted := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: sshKeySecretName, Namespace: helper.GetHostingNamespace(testHD)}}: true,
	}

	checker, err = newManifestResourceChecker(ctx, client, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")
	assert.Nil(t, checker.shouldNotHave(deleted), "err nil when all requrie resource exist in manifestwork")
}

func TestManifestWorkSecrets(t *testing.T) {

	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		client.Create(ctx, np)
		defer client.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	cpoSecret := getAwsCpoSecret(testHD)
	client.Create(ctx, cpoSecret)
	defer client.Delete(ctx, cpoSecret)

	cloudSecret := getAwsCloudCtrlSecret(testHD)
	client.Create(ctx, cloudSecret)
	defer client.Delete(ctx, cloudSecret)

	nodeSecret := getAwsNodeMgmtSecret(testHD)
	client.Create(ctx, nodeSecret)
	defer client.Delete(ctx, nodeSecret)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	manifestWorkKey := types.NamespacedName{
		Name:      generateManifestName(testHD),
		Namespace: helper.GetHostingCluster(testHD)}

	var mw workv1.ManifestWork
	err = client.Get(ctx, manifestWorkKey, &mw)
	assert.Nil(t, err, "err nil when ManifestWork found")

	codecs := serializer.NewCodecFactory(client.Scheme())
	deserializer := codecs.UniversalDeserializer()

	p := testHD.Name
	secretNames := []string{p + "-pull-secret", p + "-cpo-creds", p + "-cloud-ctrl-creds", p + "-node-mgmt-creds"}
	for _, sc := range secretNames {
		found := false
		for _, manifest := range mw.Spec.Workload.Manifests {
			var s corev1.Secret
			_, gvk, _ := deserializer.Decode(manifest.Raw, nil, &s)
			if gvk.Kind == "Secret" && s.Name == sc {
				t.Log("Correctly identified Kind: " + gvk.Kind + " Name: " + s.Name)
				found = true
				break
			}
		}
		if !found {
			t.Error("Did not find secret", sc)
		}
	}
}

func TestManifestWorkCustomSecretNames(t *testing.T) {

	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	//Customize the secret names
	testHD.Spec.HostedClusterSpec.PullSecret.Name = "my-secret-to-pull"
	testHD.Spec.HostedClusterSpec.Platform.AWS.ControlPlaneOperatorCreds.Name = "my-control"
	testHD.Spec.HostedClusterSpec.Platform.AWS.KubeCloudControllerCreds.Name = "kube-creds-for-here"
	testHD.Spec.HostedClusterSpec.Platform.AWS.NodePoolManagementCreds.Name = "node-cred-may-i-use"

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// Use a custom name for the pull secret
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret-to-pull",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}

	client.Create(ctx, pullSecret)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	manifestWorkKey := types.NamespacedName{
		Name:      generateManifestName(testHD),
		Namespace: helper.GetHostingCluster(testHD)}

	var mw workv1.ManifestWork
	err = client.Get(ctx, manifestWorkKey, &mw)
	assert.Nil(t, err, "err nil when ManifestWork found")

	codecs := serializer.NewCodecFactory(client.Scheme())
	deserializer := codecs.UniversalDeserializer()

	secretNames := []string{"my-secret-to-pull", "my-control", "kube-creds-for-here", "node-cred-may-i-use"}
	for _, sc := range secretNames {
		found := false
		for _, manifest := range mw.Spec.Workload.Manifests {
			var s corev1.Secret
			_, gvk, _ := deserializer.Decode(manifest.Raw, nil, &s)
			if gvk.Kind == "Secret" && s.Name == sc {
				t.Log("Correctly identified Kind: " + gvk.Kind + " Name: " + s.Name)
				found = true
				break
			}
		}
		if !found {
			t.Error("Did not find secret", sc)
		}
	}
}

func TestManifestWorkCustomSecretNamesWithObjectRef(t *testing.T) {

	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	//Customize the secret names
	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	hostedCluster.Spec.PullSecret.Name = "my-secret-to-pull"
	hostedCluster.Spec.Platform.AWS.ControlPlaneOperatorCreds.Name = "my-control"
	hostedCluster.Spec.Platform.AWS.KubeCloudControllerCreds.Name = "kube-creds-for-here"
	hostedCluster.Spec.Platform.AWS.NodePoolManagementCreds.Name = "node-cred-may-i-use"
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		client.Create(ctx, np)
		defer client.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// Use a custom name for the pull secret
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-secret-to-pull",
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	cpoSecret := getSecret("my-control")
	client.Create(ctx, cpoSecret)
	defer client.Delete(ctx, cpoSecret)

	cloudSecret := getSecret("kube-creds-for-here")
	client.Create(ctx, cloudSecret)
	defer client.Delete(ctx, cloudSecret)

	nodeSecret := getSecret("node-cred-may-i-use")
	client.Create(ctx, nodeSecret)
	defer client.Delete(ctx, nodeSecret)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	manifestWorkKey := types.NamespacedName{
		Name:      generateManifestName(testHD),
		Namespace: helper.GetHostingCluster(testHD)}

	var mw workv1.ManifestWork
	err = client.Get(ctx, manifestWorkKey, &mw)
	assert.Nil(t, err, "err nil when ManifestWork found")

	codecs := serializer.NewCodecFactory(client.Scheme())
	deserializer := codecs.UniversalDeserializer()

	secretNames := []string{"my-secret-to-pull", "my-control", "kube-creds-for-here", "node-cred-may-i-use"}
	for _, sc := range secretNames {
		found := false
		for _, manifest := range mw.Spec.Workload.Manifests {
			var s corev1.Secret
			_, gvk, _ := deserializer.Decode(manifest.Raw, nil, &s)
			if gvk.Kind == "Secret" && s.Name == sc {
				t.Log("Correctly identified Kind: " + gvk.Kind + " Name: " + s.Name)
				found = true
				break
			}
		}
		if !found {
			t.Error("Did not find secret", sc)
		}
	}
}

func TestManifestWorkStatusUpsertToHypershiftDeployment(t *testing.T) {
	clt := initClient()
	ctx := context.Background()

	hdr := &HypershiftDeploymentReconciler{
		Client: clt,
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"

	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	clt.Create(ctx, hostedCluster)
	defer clt.Delete(ctx, hostedCluster)

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
		clt.Create(ctx, np)
		defer clt.Delete(ctx, np)
	}
	initFakeClient(hdr, fakeObjList...)

	clt.Create(ctx, testHD)
	defer clt.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)
	clt.Create(ctx, pullSecret)
	defer clt.Delete(ctx, pullSecret)

	cpoSecret := getAwsCpoSecret(testHD)
	clt.Create(ctx, cpoSecret)
	defer clt.Delete(ctx, cpoSecret)

	cloudSecret := getAwsCloudCtrlSecret(testHD)
	clt.Create(ctx, cloudSecret)
	defer clt.Delete(ctx, cloudSecret)

	nodeSecret := getAwsNodeMgmtSecret(testHD)
	clt.Create(ctx, nodeSecret)
	defer clt.Delete(ctx, nodeSecret)

	_, err := hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successful")

	checker, err := newManifestResourceChecker(ctx, clt, getManifestWorkKey(testHD))
	assert.Nil(t, err, "err nil when the mainfestwork check created")

	assert.True(t, len(checker.spec.ManifestConfigs) != 0, "should have manifestconfigs")

	assert.Nil(t, checker.update(), "err nil when can get the hosting manifestwork")

	resStr := "test"
	trueStr := "True"
	falseStr := "False"
	msgStr := "nope"
	progress := "Partial"

	_ = falseStr

	resStr1 := "WaitingForAvailableMachines"

	manifestWork := checker.obj

	origin := manifestWork.DeepCopy()

	hcCondInput := workv1.ManifestCondition{
		ResourceMeta: workv1.ManifestResourceMeta{
			Group:     hyp.GroupVersion.Group,
			Resource:  HostedClusterResource,
			Name:      testHD.Name,
			Namespace: helper.GetHostingNamespace(testHD),
		},
		StatusFeedbacks: workv1.StatusFeedbackResult{
			Values: []workv1.FeedbackValue{
				{
					Name: Reason,
					Value: workv1.FieldValue{
						Type:   workv1.String,
						String: &resStr,
					},
				},
				{
					Name: StatusFlag,
					Value: workv1.FieldValue{
						Type:   workv1.String,
						String: &trueStr,
					},
				},
				{
					Name: Message,
					Value: workv1.FieldValue{
						Type:   workv1.String,
						String: &msgStr,
					},
				},
				{
					Name: Progress,
					Value: workv1.FieldValue{
						Type:   workv1.String,
						String: &progress,
					},
				},
			},
		},
	}

	nodepoolInput := []workv1.ManifestCondition{
		{
			ResourceMeta: workv1.ManifestResourceMeta{
				Group:     hyp.GroupVersion.Group,
				Resource:  NodePoolResource,
				Name:      testHD.Name,
				Namespace: helper.GetHostingNamespace(testHD),
			},
			StatusFeedbacks: workv1.StatusFeedbackResult{
				Values: []workv1.FeedbackValue{
					{
						Name: Reason,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &resStr,
						},
					},
					{
						Name: StatusFlag,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &trueStr,
						},
					},
					{
						Name: Message,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &msgStr,
						},
					},
				},
			},
		},

		{
			ResourceMeta: workv1.ManifestResourceMeta{
				Group:     hyp.GroupVersion.Group,
				Resource:  NodePoolResource,
				Name:      testHD.Name,
				Namespace: helper.GetHostingNamespace(testHD),
			},
			StatusFeedbacks: workv1.StatusFeedbackResult{
				Values: []workv1.FeedbackValue{
					{
						Name: Reason,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &resStr1,
						},
					},
					{
						Name: StatusFlag,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &falseStr,
						},
					},
					{
						Name: Message,
						Value: workv1.FieldValue{
							Type:   workv1.String,
							String: &msgStr,
						},
					},
				},
			},
		},
	}

	feedbackFine := workv1.ManifestResourceStatus{
		Manifests: append(nodepoolInput, hcCondInput),
	}

	manifestWork.Status.ResourceStatus = feedbackFine

	assert.Nil(t, clt.Status().Patch(ctx, manifestWork, client.MergeFrom(origin)), "err nil when update manifetwork status")

	checker.update()

	workNodepoolCond := checker.status.ResourceStatus.Manifests

	assert.Len(t, workNodepoolCond, 3, "should have 3 feedbacks")

	_, err = hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successful")

	updatedHD := &hyd.HypershiftDeployment{}
	assert.Nil(t, clt.Get(ctx, getNN, updatedHD), "err nil Get updated hypershiftDeployment")

	hcAvaCond := condmeta.FindStatusCondition(updatedHD.Status.Conditions, string(hypdeployment.HostedClusterAvailable))

	assert.NotNil(t, hcAvaCond, "not nil, should find a hostedcluster condition")
	assert.NotEmpty(t, hcAvaCond.Reason, "condition reason should be nil")

	hcProCond := condmeta.FindStatusCondition(updatedHD.Status.Conditions, string(hypdeployment.HostedClusterProgress))

	assert.NotNil(t, hcProCond, "not nil, should find a hostedcluster condition")
	assert.NotEmpty(t, hcProCond.Reason, "condition reason should be nil")

	nodepoolCond := condmeta.FindStatusCondition(updatedHD.Status.Conditions, string(hypdeployment.Nodepool))

	assert.NotNil(t, nodepoolCond, "not nil, should find a hostedcluster condition")
	assert.NotEmpty(t, nodepoolCond.Reason, "condition reason should be nil")
	assert.True(t, nodepoolCond.Reason == resStr1, "true, only contain a failed reason")
}

func TestGetManifestPayloadByName(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test1-etcd-encryption-key",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		Type: corev1.SecretTypeOpaque,
	}
	secretRaw, _ := json.Marshal(secret)
	secretManifest := workv1.Manifest{RawExtension: runtime.RawExtension{Object: secret, Raw: secretRaw}}

	secret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test2-etcd-encryption-key",
			Namespace: "test",
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		Type: corev1.SecretTypeOpaque,
	}
	secret2Raw, _ := json.Marshal(secret2)
	secret2Manifest := workv1.Manifest{RawExtension: runtime.RawExtension{Object: secret2, Raw: secret2Raw}}

	payload := []workv1.Manifest{secretManifest, secret2Manifest}
	found, _ := getManifestPayloadSecretByName(&payload, "test1-etcd-encryption-key")
	assert.Equal(t, secret, found, "found secret test1-etcd-encryption-key")

	found, _ = getManifestPayloadSecretByName(&payload, "test2-etcd-encryption-key")
	assert.Equal(t, secret2, found, "found secret test2-etcd-encryption-key")
}

func TestManifestWorkHostedClusterAttributes(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	cpoSecret := getAwsCpoSecret(testHD)
	client.Create(ctx, cpoSecret)
	defer client.Delete(ctx, cpoSecret)

	cloudSecret := getAwsCloudCtrlSecret(testHD)
	client.Create(ctx, cloudSecret)
	defer client.Delete(ctx, cloudSecret)

	nodeSecret := getAwsNodeMgmtSecret(testHD)
	client.Create(ctx, nodeSecret)
	defer client.Delete(ctx, nodeSecret)

	// Add a new attribute in the unstructured HostedCluster.Spec
	hostedCluster := getHostedClusterForManifestworkTest(testHD)
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	mapHc, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hostedCluster)
	assert.Nil(t, err, "err nil when hosted cluster is successfully converted to unstructured")
	usHcSpec := mapHc["spec"].(map[string]interface{})
	usHcSpec["new_attribute"] = "test"
	usHostedCluster := &unstructured.Unstructured{}
	usHostedCluster.Object = mapHc

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, usHostedCluster)

	// Add a new attribute in the unstructured NodePool.Spec
	nps := getNodePools(testHD)
	for _, np := range nps {
		client.Create(ctx, np)
		defer client.Delete(ctx, np)

		mapNp, err := runtime.DefaultUnstructuredConverter.ToUnstructured(np)
		assert.Nil(t, err, "err nil when node pool is successfully converted to unstructured")
		usNpSpec := mapNp["spec"].(map[string]interface{})
		usNpSpec["new_attribute"] = "test"
		usNp := &unstructured.Unstructured{}
		usNp.Object = mapNp

		fakeObjList = append(fakeObjList, usNp)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	// Check manifestwork.manifest has hostedcluster.spec with the new attribute
	manifestWork := &workv1.ManifestWork{}
	err = client.Get(ctx, getManifestWorkKey(testHD), manifestWork)
	assert.Nil(t, err, "err nil when manifestwork is created successfully")

	getPayloadInManifestwork := func(kind string) *unstructured.Unstructured {
		for _, v := range manifestWork.Spec.Workload.Manifests {
			u := &unstructured.Unstructured{}
			if err := json.Unmarshal(v.Raw, u); err != nil {
				t.Logf("failed to unmarshall hostedcluster in manifestwork: %v", err)
				return nil
			}
			if u.GetObjectKind().GroupVersionKind().Kind == kind {
				return u
			}
		}
		return nil
	}

	usHcFromManifest := getPayloadInManifestwork("HostedCluster")
	assert.NotNil(t, usHcFromManifest, "not nil when the mainfestwork contains the hosted cluster resource")
	hcSpecFromManifest := usHcFromManifest.Object["spec"].(map[string]interface{})
	assert.Equal(t, "test", hcSpecFromManifest["new_attribute"], "equals when the new attribute exists in the hosted cluster")

	// Check manifestwork.manifest has hostedcluster.spec with the new attribute
	usNpFromManifest := getPayloadInManifestwork("NodePool")
	assert.NotNil(t, usNpFromManifest, "not nil when the mainfestwork contains the node pool resource")
	npSpecFromManifest := usNpFromManifest.Object["spec"].(map[string]interface{})
	assert.Equal(t, "test", npSpecFromManifest["new_attribute"], "equals when the new attribute exists in the node pool")

	// Test HostedCluster.Spec validation - change pullSecret to invalid type
	usHcSpec["pullSecret"] = ""
	var fakeObjList2 []runtime.Object
	fakeObjList2 = append(fakeObjList2, usHostedCluster)
	fakeObjList2 = append(fakeObjList2, fakeObjList[1:]...)
	initFakeClient(hdr, fakeObjList2...)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.NotNil(t, err, "err nil when reconcile was successfull")

	var resultHD hyd.HypershiftDeployment
	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, fmt.Sprintf("HostedClusterRef %v:%v is invalid", testHD.Namespace, testHD.Spec.HostedClusterRef.Name), c.Message, "is equal when hostingCluster is invalid")
}

func TestHostedClusterAndNodePoolValidationsWithObjectRef(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"

	hostedCluster := getHostedCluster(testHD)
	client.Create(ctx, hostedCluster)
	defer client.Delete(ctx, hostedCluster)

	// Test validate - ClusterName in nodepool does not reference the hostedCluster
	npRef := []corev1.LocalObjectReference{}
	nps := getNodePools(testHD)
	for _, np := range nps {
		np.Spec.ClusterName = "wrongCluster"
		client.Create(ctx, np)
		defer client.Delete(ctx, np)

		npRef = append(npRef, corev1.LocalObjectReference{Name: np.Name})
	}

	testHD.Spec.HostedClusterRef = corev1.LocalObjectReference{Name: hostedCluster.Name}
	testHD.Spec.NodePoolsRef = npRef
	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var resultHD hyd.HypershiftDeployment
	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "incorrect Spec.ClusterName in NodePool", c.Message, "is equal when clusterName is incorrect")

	// Test validate - Platform.Type for NodePool does not match HostedCluster
	nps = getNodePools(testHD)
	for _, np := range nps {
		client.Get(ctx, types.NamespacedName{Namespace: np.Namespace, Name: np.Name}, np)
		np.Spec.Platform.Type = hyp.NonePlatform
		client.Update(ctx, np)
	}

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "Platform.Type value mismatch", c.Message, "is equal when Platform.Type is mismatched")

	// Test validate - Node Pool doesn't exist
	nps = getNodePools(testHD)
	for _, np := range nps {
		client.Get(ctx, types.NamespacedName{Namespace: np.Namespace, Name: np.Name}, np)
		err = client.Delete(ctx, np)
	}

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "nodePool not found", c.Message, "is equal when nodePool is not found")

	// Test validate - HostedCluster doesn't exist
	client.Delete(ctx, hostedCluster)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "hostedCluster not found", c.Message, "is equal when hostedCluster is not found")
}

func TestHostedClusterAndNodePoolValidations(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"
	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	pullSecret := getPullSecret(testHD)
	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	// Test validate - Release mismatch between nodepool and hostedcluster
	client.Get(ctx, types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, testHD)
	testHD.Spec.HostedClusterSpec.Release = hyp.Release{}
	testHD.Spec.NodePools[0].Spec.ClusterName = "wrongCluster"
	client.Update(ctx, testHD)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var resultHD hyd.HypershiftDeployment
	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "incorrect Spec.ClusterName in NodePool", c.Message, "is equal when Spec.ClusterName in NodePool is incorrect")

	// Test validate - Platform.Type for NodePool does not match HostedCluster
	client.Get(ctx, types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, testHD)
	testHD.Spec.NodePools[0].Spec.ClusterName = testHD.Name
	testHD.Spec.NodePools[0].Spec.Platform.Type = hyp.NonePlatform
	client.Update(ctx, testHD)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	err = hdr.Get(context.Background(), types.NamespacedName{Namespace: testHD.Namespace, Name: testHD.Name}, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, "Platform.Type value mismatch", c.Message, "is equal when Spec.ClusterName in NodePool is incorrect")
}

func TestValidateSecurityConstraints(t *testing.T) {
	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client:                  client,
		Log:                     ctrl.Log.WithName("tester"),
		ValidateClusterSecurity: false,
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	passed, err := hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when HypershiftDeploymentReconciler ValidateClusterSecurity is false")
	assert.True(t, passed, "when HypershiftDeploymentReconciler ValidateClusterSecurity is false")

	hdr.ValidateClusterSecurity = true

	passed, err = hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when validating HypershiftDeploymentReconciler security constraints")
	assert.False(t, passed, "when validating namespace needs at least one bound ManagedClusterSetBinding")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.True(t, strings.Contains(c.Message, "a bound ManagedClusterSetBinding is required in namespace"), "when validating namespace needs at least one bound ManagedClusterSetBinding")

	binding := getClusterSetBinding(testHD.Namespace)
	client.Create(ctx, binding)
	defer client.Delete(ctx, binding)

	passed, err = hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when validating HypershiftDeploymentReconciler security constraints")
	assert.False(t, passed, "when validating namespace needs at least one bound ManagedClusterSetBinding")

	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.True(t, strings.Contains(c.Message, "a bound ManagedClusterSetBinding is required in namespace"), "when validating namespace needs at least one bound ManagedClusterSetBinding")

	binding.Status = clusterv1beta1.ManagedClusterSetBindingStatus{
		Conditions: []metav1.Condition{
			{
				Type:   clusterv1beta1.ClusterSetBindingBoundType,
				Status: metav1.ConditionTrue,
			},
		},
	}
	client.Status().Update(ctx, binding)

	passed, err = hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when validating HypershiftDeploymentReconciler security constraints")
	assert.False(t, passed, "when validating ManagedCluster exist")

	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.True(t, strings.Contains(c.Message, "ManagedCluster is required"), "when validating ManagedCluster exist")

	cluster := getCluster(testHD.Spec.HostingCluster)
	client.Create(ctx, cluster)
	defer client.Delete(ctx, cluster)

	passed, err = hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when validating HypershiftDeploymentReconciler security constraints")
	assert.False(t, passed, "when validating HostingCluster needs to be a ManagedCluster that is a member of a ManagedClusterSet")

	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.WorkConfigured))
	assert.True(t, strings.Contains(c.Message, "is a member of a ManagedClusterSet"), "when validating HostingCluster needs to be a ManagedCluster that is a member of a ManagedClusterSet")

	clusterSet := getClusterSet()
	client.Create(ctx, clusterSet)
	defer client.Delete(ctx, clusterSet)

	cluster.ObjectMeta.Labels = map[string]string{
		clusterv1beta1.ClusterSetLabel: "dev",
	}
	client.Update(ctx, cluster)

	passed, err = hdr.validateSecurityConstraints(ctx, testHD)
	assert.Nil(t, err, "is nil when validating HypershiftDeploymentReconciler security constraints")
	assert.True(t, passed, "when validating HostingCluster needs to be a ManagedCluster that is a member of a ManagedClusterSet")
}

func TestDeleteManifestworkWaitCleanUp(t *testing.T) {

	client := initClient()
	ctx := context.Background()
	hdr := &HypershiftDeploymentReconciler{
		Client:                  client,
		Log:                     ctrl.Log.WithName("tester"),
		ValidateClusterSecurity: false,
	}

	testHD := getHDforManifestWork()
	testHD.Spec.HostingCluster = "local-cluster"

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	mw, _ := scaffoldManifestwork(testHD)
	client.Create(ctx, mw)
	defer client.Delete(ctx, mw)

	rqst, err := hdr.deleteManifestworkWaitCleanUp(ctx, testHD)
	assert.Nil(t, err, "is nil when deleteManifestWorkWaitCleanUp is successful")
	assert.EqualValues(t, ctrl.Result{RequeueAfter: 1 * time.Second, Requeue: true}, rqst, "request requeue should be 1s")

	err = client.Get(ctx, types.NamespacedName{Name: mw.Name, Namespace: mw.Namespace}, mw)
	mw.Status.Conditions = []metav1.Condition{
		metav1.Condition{
			Type:               string(workv1.WorkAvailable),
			ObservedGeneration: mw.Generation,
			Status:             metav1.ConditionTrue,
		},
	}
	err = client.Update(ctx, mw)
	assert.Nil(t, err, "is nil when condition is added")

	rqst, err = hdr.deleteManifestworkWaitCleanUp(ctx, testHD)
	assert.Nil(t, err, "is nil when deleteManifestWorkWaitCleanUp is successful")
	assert.EqualValues(t, ctrl.Result{RequeueAfter: 20 * time.Second, Requeue: true}, rqst, "request requeue should be 20s")
}
