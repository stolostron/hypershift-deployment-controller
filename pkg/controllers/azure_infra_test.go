package controllers

import (
	"context"
	"testing"

	hydapi "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//Does not include Location
func getFakeAzureHD() *hydapi.HypershiftDeployment {
	infraOut := getAzureInfrastructureOut()
	testHD := getHypershiftDeployment("default", "test1")

	testHD.Spec.HostingCluster = "local-cluster"
	testHD.Spec.InfraID = infraOut.InfraID
	testHD.Spec.Infrastructure.Platform = &hypdeployment.Platforms{Azure: &hypdeployment.AzurePlatform{}}
	ScaffoldAzureHostedClusterSpec(testHD, infraOut)
	ScaffoldAzureNodePoolSpec(testHD, infraOut)
	return testHD
}

func TestCreateAzureInfra(t *testing.T) {
	ctx := context.Background()
	hyd := getFakeAzureHD()
	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	corev1.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	r.Client.Create(ctx, getS3Secret("local-cluster"))
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandler{}

	t.Log("Test missing: Spec.Infrastructure.Platform.Azure.Location")
	_, err := r.createAzureInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	c := meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, when location is missing")
	assert.Equal(t, hypdeployment.MisConfiguredReason, c.Reason, "mis-configured when missing location")
	assert.Equal(t, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform.Azure.Location", c.Message, "equal when correct message is provided")

	t.Log("Test with: Spec.Infrastructure.Platform.Azure.Location")
	hyd = getFakeAzureHD()
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err = r.createAzureInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionTrue, c.Status, "true, when Azure createAzureInfra is successful")
	assert.Equal(t, hypdeployment.ConfiguredAsExpectedReason, c.Reason, "configured correctly")

	t.Log(hyd.Status.Conditions)
}

func TestCreateAzureInfraFailure(t *testing.T) {
	ctx := context.Background()
	hyd := getFakeAzureHD()
	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandlerFailure{}

	t.Log("Test with bad cloud provider secret")
	hyd = getFakeAzureHD()
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err := r.createAzureInfra(hyd, getPullSecret(hyd))
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Check provider condition
	c := meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.ProviderSecretConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, when Azure cloud provider osServicePrincipal is invalid")
	assert.Equal(t, hypdeployment.MisConfiguredReason, c.Reason, "invalid cloud provider secret")

	t.Log("Test with valid cloud provider secret, but failing AzureInfraCreator")
	hyd.Status.Conditions = nil
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err = r.createAzureInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Check provider condition
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.ProviderSecretConfigured))
	assert.Nil(t, c, "nil, when is not written")

	//Check platform condition when AzureInfraCreator fails
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, when removing the Azure infrastructure")
	assert.Equal(t, hypdeployment.MisConfiguredReason, c.Reason, "expected not to configure")
	assert.Equal(t, "failed to create azure infrastructure", c.Message, "expected message when AzureInfraCreator fails")

}

func TestDestroyAzureInfra(t *testing.T) {
	ctx := context.Background()
	hyd := getFakeAzureHD()
	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandler{}

	t.Log("Test with bad cloud provider secret")
	hyd = getFakeAzureHD()
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err := r.destroyAzureInfrastructure(hyd, getPullSecret(hyd))
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Check provider condition
	c := meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.ProviderSecretConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, when Azure cloud provider osServicePrincipal is invalid")
	assert.Equal(t, hypdeployment.MisConfiguredReason, c.Reason, "invalid cloud provider secret")

	t.Log("Test with valid cloud provider secret")
	hyd.Status.Conditions = nil
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err = r.destroyAzureInfrastructure(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Check provider condition
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.ProviderSecretConfigured))
	assert.Nil(t, c, "nil, when is not written")

	//Check platform condition when AzureInfraCreator fails
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, AzureInfraDestroyer is successful")
	assert.Equal(t, hypdeployment.PlatfromDestroyReason, c.Reason, "expected to be destroying")
	assert.Equal(t, "Removing Azure infrastructure with infra-id: test2-abcde", c.Message, "expected message when AzureInfraDestroyer is successful")

	t.Log("Test with AzureInfraDestroyer failure")
	r.InfraHandler = &FakeInfraHandlerFailure{}

	hyd.Status.Conditions = nil
	hyd.Spec.Infrastructure.Platform.Azure.Location = "centralus"
	_, err = r.destroyAzureInfrastructure(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Check platform condition when AzureInfraCreator fails
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status, "false, AzureInfraDestroyer is successful")
	assert.Equal(t, hypdeployment.PlatfromDestroyReason, c.Reason, "expected to be destroying")
	assert.Equal(t, "failed to destroy azure infrastructure", c.Message, "expected message when AzureInfraDestroyer is successful")
}
