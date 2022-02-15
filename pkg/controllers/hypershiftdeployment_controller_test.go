package controllers

import (
	"context"
	"testing"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	hyd "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"
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
		PrivateSubnetID: "privatesubnet",
		PublicSubnetID:  "publicsubnet",
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
	err := client.Create(context.Background(), ScaffoldNodePool("default", infraOut.InfraID, testHD.Spec.NodePools[0]))

	assert.Nil(t, err, "err is nil when NodePools is created successfully")
	t.Log("ScaffoldNodePool was successful")
}

func TestHypershiftdeployment_controller(t *testing.T) {

	client := initClient()

	infraOut := getInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")
	testHD.Spec.Infrastructure.Platform = &hyd.Platforms{AWS: &hyd.AWSPlatform{}}
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
