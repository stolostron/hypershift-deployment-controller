package controllers

import (
	"context"

	"fmt"
	"os"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	"github.com/openshift/hypershift/cmd/infra/azure"
	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

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

func getPullSecret(testHD *hyd.HypershiftDeployment) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-pull-secret", testHD.GetName()),
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
}

func getProviderSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "providersecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"pullSecret": []byte(`docker-pull-secret`),
		},
	}
}

// TODO Azure test using provider secret
/*func TestManifestWorkFlowProviderSecret(t *testing.T) {
	client := initClient()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.TargetManagedCluster = "local-cluster"
	testHD.Spec.Infrastructure.CloudProvider.Name = "providersecret"

	client.Create(ctx, testHD)
	defer client.Delete(ctx, testHD)

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	client.Create(ctx, getProviderSecret())

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
		Log:    ctrl.Log.WithName("tester"),
	}

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.ProviderSecretConfigured))
	t.Log("Condition msg: " + c.Message)
	assert.Equal(t, "Missing targetManagedCluster for override: MANIFESTWORK", c.Message, "is equal when targetManagedCluster is missing")
}*/
