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
	"github.com/openshift/hypershift/cmd/infra/azure"
	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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

func getAWSInfrastructureOut() *aws.CreateInfraOutput {
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

func getAzureInfrastructureOut() *azure.CreateInfraOutput {
	return &azure.CreateInfraOutput{
		BaseDomain:        "my-domain.com",
		PublicZoneID:      "12345",
		PrivateZoneID:     "67890",
		Location:          "abcde",
		ResourceGroupName: "default",
		VNetID:            "12345abcde",
		VnetName:          "vnet-name",
		SubnetName:        "subnet-name",
		BootImageID:       "image-12345",
		InfraID:           "test2-abcde",
		MachineIdentityID: "12345",
		SecurityGroupName: "sg-name",
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

func TestScaffoldAWSHostedClusterSpec(t *testing.T) {

	t.Log("Test AWS scaffolding")
	testHD := getHypershiftDeployment("default", "test1")

	// The Reconcile code exits with a condition if platform or AWS are nil
	oAWS := getAWSInfrastructureOut()
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{Region: oAWS.Region}}
	assert.Nil(t, testHD.Spec.HostedClusterSpec, "HostedClusterSpec is nil")

	ScaffoldAWSHostedClusterSpec(testHD, oAWS)
	//assert.Equal(t, oAWS.InfraID, testHD.Spec.HostedClusterSpec.InfraID, "InfraID should be "+oAWS.InfraID)
	assert.Equal(t, oAWS.Region, testHD.Spec.HostedClusterSpec.Platform.AWS.Region, "Region should be "+oAWS.Region)
	assert.Equal(t, oAWS.VPCID, testHD.Spec.HostedClusterSpec.Platform.AWS.CloudProviderConfig.VPC, "VPCID should be "+oAWS.VPCID)
	//Skipped Zones
	assert.Equal(t, oAWS.ComputeCIDR, testHD.Spec.HostedClusterSpec.Networking.MachineCIDR, "ComputeCIDR should be "+oAWS.ComputeCIDR)
	assert.Equal(t, oAWS.BaseDomain, testHD.Spec.HostedClusterSpec.DNS.BaseDomain, "BaseDomain should be "+oAWS.BaseDomain)
	assert.Equal(t, oAWS.PrivateZoneID, testHD.Spec.HostedClusterSpec.DNS.PrivateZoneID, "PrivateZoneID should be "+oAWS.PrivateZoneID)
	assert.Equal(t, oAWS.PublicZoneID, testHD.Spec.HostedClusterSpec.DNS.PublicZoneID, "PublicZoneID should be "+oAWS.PublicZoneID)
	// Skiped LocalZoneID
}

func TestScaffoldAzureHostedClusterSpec(t *testing.T) {

	t.Log("Test Azure scaffolding")
	testHD := getHypershiftDeployment("default", "test1")

	// The Reconcile code exits with a condition if platform or AWS are nil
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{Azure: &hyd.AzurePlatform{}}
	assert.Nil(t, testHD.Spec.HostedClusterSpec, "HostedClusterSpec is nil")

	oAzure := getAzureInfrastructureOut()
	ScaffoldAzureHostedClusterSpec(testHD, oAzure)
	//assert.Equal(t, oAzure.InfraID, testHD.Spec.HostedClusterSpec.InfraID, "InfraID should be "+oAzure.InfraID)
	assert.Equal(t, oAzure.BaseDomain, testHD.Spec.HostedClusterSpec.DNS.BaseDomain, "BaseDomain should be "+oAzure.BaseDomain)
	assert.Equal(t, oAzure.PublicZoneID, testHD.Spec.HostedClusterSpec.DNS.PublicZoneID, "PublicZoneID should be "+oAzure.PublicZoneID)
	assert.Equal(t, oAzure.PrivateZoneID, testHD.Spec.HostedClusterSpec.DNS.PrivateZoneID, "PrivateZoneID should be "+oAzure.PrivateZoneID)
	assert.Equal(t, oAzure.Location, testHD.Spec.HostedClusterSpec.Platform.Azure.Location, "Location should be "+oAzure.Location)
	assert.Equal(t, oAzure.ResourceGroupName, testHD.Spec.HostedClusterSpec.Platform.Azure.ResourceGroupName, "ResourceGroupName should be "+oAzure.ResourceGroupName)
	assert.Equal(t, oAzure.VNetID, testHD.Spec.HostedClusterSpec.Platform.Azure.VnetID, "VNetID should be "+oAzure.VNetID)
	assert.Equal(t, oAzure.VnetName, testHD.Spec.HostedClusterSpec.Platform.Azure.VnetName, "VnetName should be "+oAzure.VnetName)
	assert.Equal(t, oAzure.SubnetName, testHD.Spec.HostedClusterSpec.Platform.Azure.SubnetName, "SubnetName should be "+oAzure.SubnetName)
	assert.Equal(t, oAzure.MachineIdentityID, testHD.Spec.HostedClusterSpec.Platform.Azure.MachineIdentityID, "MachineIdentityID should be "+oAzure.MachineIdentityID)
	assert.Equal(t, oAzure.SecurityGroupName, testHD.Spec.HostedClusterSpec.Platform.Azure.SecurityGroupName, "SecurityGroupName should be "+oAzure.SecurityGroupName)
	assert.Equal(t, oAzure.SubnetName, testHD.Spec.HostedClusterSpec.Platform.Azure.SubnetName, "SubnetName should be "+oAzure.SubnetName)
}

func TestScaffoldAWSHostedCluster(t *testing.T) {
	testHD := getHypershiftDeployment("default", "test1")

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	ScaffoldAWSHostedClusterSpec(testHD, getAWSInfrastructureOut())

	client := initClient()
	err := client.Create(context.Background(), ScaffoldHostedCluster(testHD))

	assert.Nil(t, err, "err is nil when HostedCluster Custom Resource is well formed")
	t.Log("ScaffoldHostedCluster was successful")
}

func TestScaffoldAzureHostedCluster(t *testing.T) {
	testHD := getHypershiftDeployment("default", "test1")

	ScaffoldAzureHostedClusterSpec(testHD, getAzureInfrastructureOut())

	client := initClient()
	err := client.Create(context.Background(), ScaffoldHostedCluster(testHD))

	assert.Nil(t, err, "err is nil when HostedCluster Custom Resource is well formed")
	t.Log("ScaffoldHostedCluster was successful")
}

func TestScaffoldAWSNodePoolSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	assert.Equal(t, 0, len(testHD.Spec.NodePools), "Should be zero node pools")
	oAWS := getAWSInfrastructureOut()
	ScaffoldAWSNodePoolSpec(testHD, oAWS)
	assert.Equal(t, 1, len(testHD.Spec.NodePools), "Should be 1 node pools")
	assert.Equal(t, oAWS.SecurityGroupID, *testHD.Spec.NodePools[0].Spec.Platform.AWS.SecurityGroups[0].ID, "SecurityGroupID is equal")
	assert.Equal(t, oAWS.Zones[0].SubnetID, *testHD.Spec.NodePools[0].Spec.Platform.AWS.Subnet.ID, "SubnetID is equal")
}

func TestScaffoldAzureNodePoolSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	assert.Equal(t, 0, len(testHD.Spec.NodePools), "Should be zero node pools")
	oAzure := getAzureInfrastructureOut()
	ScaffoldAzureNodePoolSpec(testHD, oAzure)
	assert.Equal(t, 1, len(testHD.Spec.NodePools), "Should be 1 node pools")
	assert.Equal(t, oAzure.BootImageID, testHD.Spec.NodePools[0].Spec.Platform.Azure.ImageID, "ImageID is equal")
}

func TestScaffoldAWSNodePool(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	infraOut := getAWSInfrastructureOut()
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

	client := initClient()
	err := client.Create(context.Background(), ScaffoldNodePool(testHD, testHD.Spec.NodePools[0]))

	assert.Nil(t, err, "err is nil when NodePools is created successfully")
	t.Log("ScaffoldNodePool was successful")
}

func TestScaffoldAzureNodePool(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1")

	infraOut := getAzureInfrastructureOut()
	ScaffoldAzureNodePoolSpec(testHD, infraOut)

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
// and reference secret is put into manifestwork payload
func TestManifestWorkFlow(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Override = hyd.InfraConfigureWithManifest

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	testHD.Spec.InfraID = infraOut.InfraID
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

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
		Namespace: helper.GetTargetManagedCluster(testHD)}

	manifestWork := &workv1.ManifestWork{}

	err = client.Get(ctx, manifestWorkKey, manifestWork)
	assert.Nil(t, err, "err nil when manifestwork generated")

	requiredResource := map[kindAndKey]bool{
		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Namespace"},
			NamespacedName: types.NamespacedName{
				Name: "default", Namespace: ""}}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "HostedCluster"},
			NamespacedName: genKeyFromObject(testHD)}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "hypershift.openshift.io", Version: "v1alpha1", Kind: "NodePool"},
			NamespacedName: genKeyFromObject(testHD)}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: helper.GetTargetNamespace(testHD)}}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-cpo-creds", Namespace: helper.GetTargetNamespace(testHD)}}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-node-mgmt-creds", Namespace: helper.GetTargetNamespace(testHD)}}: false,

		{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: "test1-pull-secret", Namespace: helper.GetTargetNamespace(testHD)}}: false,
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

	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Override = hyd.InfraConfigureWithManifest

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	testHD.Spec.InfraID = infraOut.InfraID
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

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
		Namespace: helper.GetTargetManagedCluster(testHD)}

	manifestWork := &workv1.ManifestWork{}

	err = client.Get(ctx, manifestWorkKey, manifestWork)
	assert.Nil(t, err, "err nil when manifestwork generated")

	requiredResource := map[kindAndKey]bool{
		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgSecretName, Namespace: helper.GetTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "Secret"},
			NamespacedName: types.NamespacedName{
				Name: cfgItemSecretName, Namespace: helper.GetTargetNamespace(testHD)}}: false,

		kindAndKey{
			GroupVersionKind: schema.GroupVersionKind{
				Group: "", Version: "v1", Kind: "ConfigMap"},
			NamespacedName: types.NamespacedName{
				Name: cfgConfigName, Namespace: helper.GetTargetNamespace(testHD)}}: false,
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

func TestHypershiftdeployment_controller(t *testing.T) {

	client := initClient()

	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

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

func TestConfigureFalseWithManifestWork(t *testing.T) {

	client := initClient()

	testHD := getHypershiftDeployment(getNN.Namespace, getNN.Name)
	testHD.Spec.Override = hyd.InfraConfigureWithManifest
	testHD.Spec.TargetManagedCluster = "local-host"
	testHD.Spec.TargetNamespace = "multicluster-engine"

	client.Create(context.Background(), testHD)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
	}
	_, err := hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successful")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")
	assert.True(t, meta.IsStatusConditionFalse(resultHD.Status.Conditions, string(hyd.WorkConfigured)), "is true when ManifestWork is not configured correctly")

	err = client.Delete(context.Background(), &resultHD)
	assert.Nil(t, err, "is nill when HypershiftDeployment resource is deleted")

	_, err = hdr.Reconcile(context.Background(), ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile on delete was successful")

	err = client.Get(context.Background(), getNN, &resultHD)
	assert.True(t, errors.IsNotFound(err), "is not found when HypershiftDeployment resource is deleted successfully")
}
