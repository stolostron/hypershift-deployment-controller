package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func getHypershiftDeployment(namespace string, name string) *hyd.HypershiftDeployment {
	return &hyd.HypershiftDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: hyd.HypershiftDeploymentSpec{
			Infrastructure: hyd.InfraSpec{
				Configure: false,
			},
		},
	}
}

func getInfrastructureOut() *aws.CreateInfraOutput {
	return &aws.CreateInfraOutput{
		Region:          "us-east-1",
		Zone:            "us-east-1a",
		InfraID:         "test1-abcde",
		ComputeCIDR:     "cidr",
		VPCID:           "vpc-id",
		Zones:           []*aws.CreateInfraOutputZone{&aws.CreateInfraOutputZone{SubnetID: "subnet-12345"}},
		SecurityGroupID: "sg-123456789",
		Name:            "test1",
		BaseDomain:      "my-domain.com",
		PublicZoneID:    "12345",
		PrivateZoneID:   "67890",
		LocalZoneID:     "abcde",
	}
}

func initClient() client.Client {
	scheme := runtime.NewScheme()
	hyd.AddToScheme(scheme)
	hyp.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	workv1.AddToScheme(scheme)

	var logger logr.Logger

	zapLog, err := zap.NewDevelopment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start up logger %v\n", err)
		os.Exit(1)
	}

	logger = zapr.NewLogger(zapLog)

	logf.SetLogger(logger)

	ncb := fake.NewClientBuilder()
	ncb.WithScheme(scheme)
	return ncb.Build()

}

var getNN = types.NamespacedName{
	Namespace: "default",
	Name:      "test1",
}

func TestScaffoldHostedClusterSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	infraOut := getInfrastructureOut()
	// The Reconcile code exits with a condition if platform or AWS are nil
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	assert.Nil(t, testHD.Spec.HostedClusterSpec, "HostedClusterSpec is nil")
	ScaffoldHostedClusterSpec(testHD, infraOut)
	assert.Equal(t, infraOut.ComputeCIDR, testHD.Spec.HostedClusterSpec.Networking.MachineCIDR, "InfraID should be "+infraOut.InfraID)
}

func TestScaffoldHostedCluster(t *testing.T) {
	testHD := getHypershiftDeployment("default", "test1")

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	ScaffoldHostedClusterSpec(testHD, getInfrastructureOut())

	client := initClient()
	err := client.Create(context.Background(), ScaffoldHostedCluster(testHD))

	assert.Nil(t, err, "err is nil when HostedCluster Custom Resource is well formed")
	t.Log("ScaffoldHostedCluster was successful")
}

func TestScaffoldNodePoolSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	assert.Equal(t, 0, len(testHD.Spec.NodePools), "Should be zero node pools")
	ScaffoldNodePoolSpec(testHD, getInfrastructureOut())
	assert.Equal(t, 1, len(testHD.Spec.NodePools), "Should be 1 node pools")
}

func TestScaffoldNodePool(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	infraOut := getInfrastructureOut()
	ScaffoldNodePoolSpec(testHD, infraOut)

	client := initClient()
	err := client.Create(context.Background(), ScaffoldNodePool(testHD, testHD.Spec.NodePools[0]))

	assert.Nil(t, err, "err is nil when NodePools is created successfully")
	t.Log("ScaffoldNodePool was successful")
}

func genKeyFromObject(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

type kindAndKey struct {
	schema.GroupVersionKind
	types.NamespacedName
}

// TestManifestWorkFlow tests when override is set to manifestwork, test if the manifestwork is created
// and referenece secret is put into manifestwork payload
func TestManifestWorkFlow(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	infraOut := getInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Override = hyd.InfraConfigureWithManifest

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	testHD.Spec.InfraID = infraOut.InfraID
	ScaffoldHostedClusterSpec(testHD, infraOut)
	ScaffoldNodePoolSpec(testHD, infraOut)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", testHD.GetName()),
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
		Namespace: getTargetManagedCluster(testHD)}

	manifestWork := &workv1.ManifestWork{}

	err = client.Get(ctx, manifestWorkKey, manifestWork)
	assert.Nil(t, err, "err nil when manifestwork generated")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "HostedCluster"},
			NamespacedName: genKeyFromObject(testHD)}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "NodePool"},
			NamespacedName: genKeyFromObject(testHD)}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: getTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-cpo-creds", Namespace: getTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: getTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-pull-secret", Namespace: getTargetNamespace(testHD)}}: false,
	}

	wl := manifestWork.Spec.Workload.Manifests

	for _, w := range wl {
		u := &unstructured.Unstructured{}
		if err := json.Unmarshal(w.Raw, u); err != nil {
			assert.Nil(t, err, "err nil when convert manifest to unstructured")
		}

		k := kindAndKey{
			GroupVersionKind: u.GetObjectKind().GroupVersionKind(),
			NamespacedName:   genKeyFromObject(u),
		}

		requiredResource[k] = true
	}

	for k, v := range requiredResource {
		assert.True(t, v, fmt.Sprintf("resource %s should be in the manifestWork.Spec.Workload.Manifests", k))
	}
}

// TestManifestWorkFlowWithExtraConfigurations test
// when override is set to manifestwork, test if the manifestwork is created
// and extra secret/configmap is put into manifestwork payload in addition to
// the required resource of TestManifestWorkFlow
func TestManifestWorkFlowWithExtraConfigurations(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	infraOut := getInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Override = hyd.InfraConfigureWithManifest

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	testHD.Spec.InfraID = infraOut.InfraID
	ScaffoldHostedClusterSpec(testHD, infraOut)
	ScaffoldNodePoolSpec(testHD, infraOut)

	cfgSecretName := "hostedcluster-config-secret-1"
	cfgConfigName := "hostedcluster-config-configmap-1"
	cfgItemSecretName := "hostedcluster-config-item-1"

	insertConfigSecretAndConfigMap := func() {
		testHD.Spec.HostedClusterSpec.Configuration = &hyp.ClusterConfiguration{}
		testHD.Spec.HostedClusterSpec.Configuration.SecretRefs = []corev1.LocalObjectReference{
			corev1.LocalObjectReference{Name: cfgSecretName}}

		testHD.Spec.HostedClusterSpec.Configuration.ConfigMapRefs = []corev1.LocalObjectReference{
			corev1.LocalObjectReference{Name: cfgConfigName}}

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
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", testHD.GetName()),
			Namespace: testHD.GetNamespace(),
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}

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

	client.Create(ctx, cm)
	defer client.Delete(ctx, cm)

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	manifestWorkKey := types.NamespacedName{
		Name:      generateManifestName(testHD),
		Namespace: getTargetManagedCluster(testHD)}

	manifestWork := &workv1.ManifestWork{}

	err = client.Get(ctx, manifestWorkKey, manifestWork)
	assert.Nil(t, err, "err nil when manifestwork generated")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgSecretName, Namespace: getTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgItemSecretName, Namespace: getTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgConfigName, Namespace: getTargetNamespace(testHD)}}: false,
	}

	wl := manifestWork.Spec.Workload.Manifests

	for _, w := range wl {
		u := &unstructured.Unstructured{}
		if err := json.Unmarshal(w.Raw, u); err != nil {
			assert.Nil(t, err, "err nil when convert manifest to unstructured")
		}

		k := kindAndKey{
			GroupVersionKind: u.GetObjectKind().GroupVersionKind(),
			NamespacedName:   genKeyFromObject(u),
		}

		requiredResource[k] = true

		t.Log(u)
	}

	for k, v := range requiredResource {
		assert.True(t, v, fmt.Sprintf("resource %s should be in the manifestWork.Spec.Workload.Manifests", k))
	}
}

func TestHypershiftdeployment_controller(t *testing.T) {

	client := initClient()

	infraOut := getInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	ScaffoldHostedClusterSpec(testHD, infraOut)
	ScaffoldNodePoolSpec(testHD, infraOut)

	client.Create(context.Background(), testHD)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
	}
	_, err := hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var testHC hyp.HostedCluster
	err = client.Get(context.Background(), getNN, &testHC)
	assert.Nil(t, err, "err is Nil when HostedCluster is successfully created")

	var testNP hyp.NodePool
	err = client.Get(context.Background(), getNN, &testNP)
	assert.Nil(t, err, "err is Nil when NodePool is successfully created")
}
