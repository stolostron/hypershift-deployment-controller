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
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func getHypershiftDeployment(namespace string, name string, configure bool) *hyd.HypershiftDeployment {
	return &hyd.HypershiftDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"test1": "doNotTransfer1",
				"test2": "doNotTransfer2",
			},
		},
		Spec: hyd.HypershiftDeploymentSpec{
			Infrastructure: hyd.InfraSpec{
				Configure: configure,
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
	clusterv1.AddToScheme(scheme)
	clusterv1beta1.AddToScheme(scheme)

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

func scaffoldTestNodePool(hyd *hypdeployment.HypershiftDeployment, npName string, npSpec hyp.NodePoolSpec) *hyp.NodePool {
	return &hyp.NodePool{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: hyp.GroupVersion.String(),
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      npName,
			Namespace: helper.GetHostingNamespace(hyd),
			Labels: map[string]string{
				constant.AutoInfraLabelName: hyd.Spec.InfraID,
			},
		},
		Spec: npSpec,
	}
}

func TestScaffoldAWSHostedClusterSpec(t *testing.T) {

	t.Log("Test AWS scaffolding")
	testHD := getHypershiftDeployment("default", "test1", false)

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
	testHD := getHypershiftDeployment("default", "test1", false)

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
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	testHD := getHypershiftDeployment("default", "test1", false)
	testHD.Spec.Infrastructure.Configure = true

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	ScaffoldAWSHostedClusterSpec(testHD, getAWSInfrastructureOut())

	client := initClient()
	hc, _ := r.scaffoldHostedCluster(ctx, testHD)
	err := client.Create(context.Background(), hc)

	assert.Nil(t, err, "err is nil when HostedCluster Custom Resource is well formed")
	t.Log("ScaffoldHostedCluster was successful")
}

func TestScaffoldAzureHostedCluster(t *testing.T) {
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	testHD := getHypershiftDeployment("default", "test1", true)

	ScaffoldAzureHostedClusterSpec(testHD, getAzureInfrastructureOut())

	client := initClient()
	hc, _ := r.scaffoldHostedCluster(ctx, testHD)
	err := client.Create(context.Background(), hc)

	assert.Nil(t, err, "err is nil when HostedCluster Custom Resource is well formed")
	t.Log("ScaffoldHostedCluster was successful")
}

func TestScaffoldAWSNodePoolSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1", true)

	assert.Equal(t, 0, len(testHD.Spec.NodePools), "Should be zero node pools")
	oAWS := getAWSInfrastructureOut()
	ScaffoldAWSNodePoolSpec(testHD, oAWS)
	assert.Equal(t, 1, len(testHD.Spec.NodePools), "Should be 1 node pools")
	assert.Equal(t, oAWS.SecurityGroupID, *testHD.Spec.NodePools[0].Spec.Platform.AWS.SecurityGroups[0].ID, "SecurityGroupID is equal")
	assert.Equal(t, oAWS.Zones[0].SubnetID, *testHD.Spec.NodePools[0].Spec.Platform.AWS.Subnet.ID, "SubnetID is equal")
}

func TestScaffoldAzureNodePoolSpec(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1", false)

	assert.Equal(t, 0, len(testHD.Spec.NodePools), "Should be zero node pools")
	oAzure := getAzureInfrastructureOut()
	ScaffoldAzureNodePoolSpec(testHD, oAzure)
	assert.Equal(t, 1, len(testHD.Spec.NodePools), "Should be 1 node pools")
	assert.Equal(t, oAzure.BootImageID, testHD.Spec.NodePools[0].Spec.Platform.Azure.ImageID, "ImageID is equal")
}

func TestScaffoldAWSNodePool(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1", false)

	infraOut := getAWSInfrastructureOut()
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

	client := initClient()
	err := client.Create(context.Background(), scaffoldTestNodePool(testHD, testHD.Spec.NodePools[0].Name, testHD.Spec.NodePools[0].Spec))

	assert.Nil(t, err, "err is nil when NodePools is created successfully")
	t.Log("ScaffoldNodePool was successful")
}

func TestScaffoldAzureNodePool(t *testing.T) {

	testHD := getHypershiftDeployment("default", "test1", false)

	infraOut := getAzureInfrastructureOut()
	ScaffoldAzureNodePoolSpec(testHD, infraOut)

	client := initClient()
	err := client.Create(context.Background(), scaffoldTestNodePool(testHD, testHD.Spec.NodePools[0].Name, testHD.Spec.NodePools[0].Spec))

	assert.Nil(t, err, "err is nil when NodePools is created successfully")
	t.Log("ScaffoldNodePool was successful")
}

func genKeyFromObject(obj metav1.Object) types.NamespacedName {
	return types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
}

//Hypershiftdeployment reconcile will error out when ther Hostingcluster is empty and the error is update to
//HypershiftDeployment.Status
//Once the hostingcluster is updated, the manifestworkwould be created.
func TestHypershiftdeployment_controller(t *testing.T) {

	client := initClient()

	ctx := context.Background()
	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1", false)
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

	client.Create(ctx, testHD)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
	}
	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var updated hyd.HypershiftDeployment
	err = client.Get(ctx, getNN, &updated)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(updated.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, constant.HostingClusterMissing, c.Message, "is equal when hostingCluster is missing")

	key := getManifestWorkKey(testHD)
	manifestwork := &workv1.ManifestWork{}
	assert.NotNil(t, client.Get(context.Background(), key, manifestwork), "err is Nil when Manifestwork is successfully created")
	updated.Spec.HostingCluster = "local-cluster"
	err = client.Update(context.Background(), &updated)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is updated")

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)

	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	assert.NotNil(t, client.Get(context.Background(), key, manifestwork), "err is Nil when Manifestwork is successfully created")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	t.Log("Check infraID label")
	assert.NotEmpty(t, resultHD.Labels, "The infra-id should always be written to the label hypershift.openshift.io/infra-id")
	assert.Contains(t, resultHD.Labels[constant.InfraLabelName], testHD.Name+"-", "The infra-id must contain the cluster name")
}

func TestHypershiftdeployment_controllerWithObjectRef(t *testing.T) {

	client := initClient()
	ctx := context.Background()

	infraOut := getAWSInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1", false)

	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
	testHD.Spec.Credentials = &hyd.CredentialARNs{AWS: &hyd.AWSCredentials{}}
	ScaffoldAWSHostedClusterSpec(testHD, infraOut)
	ScaffoldAWSNodePoolSpec(testHD, infraOut)

	hdr := &HypershiftDeploymentReconciler{
		Client: client,
	}

	// Add hosted cluster and nodepool to fake dynamic client
	var fakeObjList []runtime.Object
	hostedCluster := getHostedCluster(testHD)
	fakeObjList = append(fakeObjList, hostedCluster)

	nps := getNodePools(testHD)
	for _, np := range nps {
		fakeObjList = append(fakeObjList, np)
	}
	initFakeClient(hdr, fakeObjList...)

	client.Create(ctx, testHD)

	_, err := hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	var updated hyd.HypershiftDeployment
	err = client.Get(ctx, getNN, &updated)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	c := meta.FindStatusCondition(updated.Status.Conditions, string(hyd.WorkConfigured))
	assert.Equal(t, constant.HostingClusterMissing, c.Message, "is equal when hostingCluster is missing")

	key := getManifestWorkKey(testHD)
	manifestwork := &workv1.ManifestWork{}
	assert.NotNil(t, client.Get(context.Background(), key, manifestwork), "err is Nil when Manifestwork is successfully created")
	updated.Spec.HostingCluster = "local-cluster"
	err = client.Update(context.Background(), &updated)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is updated")

	// ensure the pull secret exist in cluster
	// this pull secret is generated by the hypershift operator
	pullSecret := getPullSecret(testHD)

	client.Create(ctx, pullSecret)
	defer client.Delete(ctx, pullSecret)

	_, err = hdr.Reconcile(ctx, ctrl.Request{NamespacedName: getNN})
	assert.Nil(t, err, "err nil when reconcile was successfull")

	assert.NotNil(t, client.Get(context.Background(), key, manifestwork), "err is Nil when Manifestwork is successfully created")

	var resultHD hyd.HypershiftDeployment
	err = client.Get(context.Background(), getNN, &resultHD)
	assert.Nil(t, err, "is nil when HypershiftDeployment resource is found")

	t.Log("Check infraID label")
	assert.NotEmpty(t, resultHD.Labels, "The infra-id should always be written to the label hypershift.openshift.io/infra-id")
	assert.Contains(t, resultHD.Labels[constant.InfraLabelName], testHD.Name+"-", "The infra-id must contain the cluster name")
}

func TestConfigureFalseWithManifestWork(t *testing.T) {

	client := initClient()

	testHD := getHypershiftDeployment(getNN.Namespace, getNN.Name, false)
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"
	testHD.Spec.InfraID = getNN.Name + "-AB1YZ"

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

	t.Log("Check infraID label")
	assert.NotEmpty(t, resultHD.Labels, "The infra-id should always be written to the label hypershift.openshift.io/infra-id")
	assert.Equal(t, resultHD.Labels[constant.InfraLabelName], testHD.Spec.InfraID, "The infra-id must contain the cluster name")
}

func TestConfigureFalseWithManifestWorkWithObjectRef(t *testing.T) {

	client := initClient()

	testHD := getHypershiftDeployment(getNN.Namespace, getNN.Name, false)
	testHD.Spec.HostingCluster = "local-host"
	testHD.Spec.HostingNamespace = "multicluster-engine"
	testHD.Spec.InfraID = getNN.Name + "-AB1YZ"

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

	t.Log("Check infraID label")
	assert.NotEmpty(t, resultHD.Labels, "The infra-id should always be written to the label hypershift.openshift.io/infra-id")
	assert.Equal(t, resultHD.Labels[constant.InfraLabelName], testHD.Spec.InfraID, "The infra-id must contain the cluster name")

	// Check PlatformConfigure and PlatformIAMConfigure status conditions
	c := meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.PlatformConfigured))
	assert.Equal(t, hypdeployment.NotApplicableReason, c.Reason, "is equal when Platform configure status condition is correct")

	c = meta.FindStatusCondition(resultHD.Status.Conditions, string(hyd.PlatformIAMConfigured))
	assert.Equal(t, hypdeployment.NotApplicableReason, c.Reason, "is equal when Platform IAM configure status condition is correct")

}

func getSecret(secretName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "default",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`docker-pull-secret`),
		},
	}
}

func getPullSecret(testHD *hyd.HypershiftDeployment) *corev1.Secret {
	return getSecret(fmt.Sprintf("%s-pull-secret", testHD.GetName()))
}

func getAwsCpoSecret(testHD *hyd.HypershiftDeployment) *corev1.Secret {
	return getSecret(fmt.Sprintf("%s-cpo-creds", testHD.GetName()))
}

func getAwsCloudCtrlSecret(testHD *hyd.HypershiftDeployment) *corev1.Secret {
	return getSecret(fmt.Sprintf("%s-cloud-ctrl-creds", testHD.GetName()))
}

func getAwsNodeMgmtSecret(testHD *hyd.HypershiftDeployment) *corev1.Secret {
	return getSecret(fmt.Sprintf("%s-node-mgmt-creds", testHD.GetName()))
}

func getProviderSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "providersecret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"pullSecret":              []byte(`docker-pull-secret`),
			"osServicePrincipal.json": []byte(`{"clientId":"00000000-0000-0000-0000-000000000000","clientSecret":"abcdef123456","tenantId":"00000000-0000-0000-0000-000000000000","subscriptionId":"00000000-0000-0000-0000-000000000000"}`),
		},
	}
}

func TestHypershiftDeploymentToHostedClusterAnnotationTransfer(t *testing.T) {
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	testHD := getHDforManifestWork()
	testHD.Spec.Infrastructure.Configure = true
	_, found := testHD.Annotations["test1"]
	assert.Equal(t, true, found, "validating annotation is present")

	_, found = testHD.Annotations["test2"]
	assert.Equal(t, true, found, "validating annotation is present")

	// Add all known annotations
	for a := range checkHostedClusterAnnotations {
		testHD.Annotations[a] = "value--" + a
	}

	resultHC, _ := r.scaffoldHostedCluster(ctx, testHD)

	// Validate all known annotations
	for a := range checkHostedClusterAnnotations {
		assert.EqualValues(t, "value--"+a, resultHC.GetAnnotations()[a], "Equal when annotation copied")
	}

	// Make sure test1 and test2 annotations were not transferred to the hostedCluster
	_, found = resultHC.GetAnnotations()["test1"]
	assert.NotEqual(t, true, found, "validating annotation is present")

	_, found = resultHC.GetAnnotations()["test2"]
	assert.NotEqual(t, true, found, "validating annotation is present")

}

func TestHypershiftDeploymentToHostedRefClusterAnnotationTransfer(t *testing.T) {
	r := GetHypershiftDeploymentReconciler()
	ctx := context.Background()

	testHD := getHDforManifestWork()

	hostedCluster := getHostedCluster(testHD)
	hostedCluster.Annotations = make(map[string]string)
	hostedCluster.Annotations[hyp.DisablePKIReconciliationAnnotation] = "value1"
	hostedCluster.Annotations["test2"] = "value2"

	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)
	initFakeClient(r, fakeObjList...)

	resultHC, _ := r.scaffoldHostedCluster(ctx, testHD)

	// Make sure test1 and test2 annotations were transferred to the hostedCluster
	found := resultHC.GetAnnotations()[hyp.DisablePKIReconciliationAnnotation]
	assert.Equal(t, found, "value1")

	found = resultHC.GetAnnotations()["test2"]
	assert.Equal(t, found, "")
}

// TODO Azure test using provider secret

func TestLocalObjectReferencesForHCandNP(t *testing.T) {
	client := initClient()
	r := &HypershiftDeploymentReconciler{
		Client: client,
	}
	ctx := context.Background()

	// HD with configure=F
	testHD := getHypershiftDeployment("default", "ObjRefTest", false)
	testHD.Spec.HostingCluster = "local-cluster"
	testHD.Spec.HostingNamespace = "clusters"
	infraOut := getAWSInfrastructureOut()
	testHD.Spec.InfraID = infraOut.InfraID

	hostedCluster := &hyp.HostedCluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "HostedCluster",
			APIVersion: "hypershift.openshift.io/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testHostedCluster",
			Namespace: testHD.Namespace,
		},
		Spec: hyp.HostedClusterSpec{
			Platform: hyp.PlatformSpec{
				Type: hyp.AWSPlatform,
			},
			Networking: hyp.ClusterNetworking{
				NetworkType: hyp.OpenShiftSDN,
			},
			Services: []hyp.ServicePublishingStrategyMapping{},
			Release: hyp.Release{
				Image: constant.ReleaseImage,
			},
			Etcd: hyp.EtcdSpec{
				ManagementType: hyp.Managed,
			},
		},
	}
	var fakeObjList []runtime.Object
	fakeObjList = append(fakeObjList, hostedCluster)

	np := &hyp.NodePool{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testNodePool",
			Namespace: testHD.Namespace,
		},
		Spec: hyp.NodePoolSpec{
			ClusterName: testHD.Name,
			Release: hyp.Release{
				Image: constant.ReleaseImage,
			},
			Platform: hyp.NodePoolPlatform{
				Type: hyp.AWSPlatform,
				AWS:  &hyp.AWSNodePoolPlatform{},
			},
			Management:       hyp.NodePoolManagement{},
			Config:           []corev1.LocalObjectReference{},
			NodeDrainTimeout: &metav1.Duration{},
		},
	}
	fakeObjList = append(fakeObjList, np)
	initFakeClient(r, fakeObjList...)

	testHD.Spec.HostedClusterRef = corev1.LocalObjectReference{Name: hostedCluster.Name}
	testHD.Spec.NodePoolsRef = []corev1.LocalObjectReference{{Name: np.Name}}

	m, err := scaffoldManifestwork(testHD)
	assert.Nil(t, err, "is nil if scaffold manifestwork successfully")
	payload := &m.Spec.Workload.Manifests

	// Manifestwork payload has hosted cluster added from hypD hostedClusterRef
	r.appendHostedCluster(ctx)(testHD, payload)
	hc := getHostedClusterInManifestPayload(payload)
	assert.NotNil(t, hc, "hostedcluster is added in manifestwork from hypershiftdeployment hostedClusterRef")
	assert.Equal(t, hc.Namespace, testHD.Spec.HostingNamespace)

	// Manifestwork payload has hosted cluster added from hypD nodePoolRef
	r.appendNodePool(ctx)(testHD, payload)
	nps := getNodePoolsInManifestPayload(payload)
	assert.Len(t, nps, 1, "nodepool is added in manifestwork from hypershiftdeployment nodePoolRef")
	assert.Equal(t, nps[0].GetNamespace(), testHD.Spec.HostingNamespace)
}
