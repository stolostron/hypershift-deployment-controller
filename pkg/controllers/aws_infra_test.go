package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	hydapi "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
)

var s = clientgoscheme.Scheme

func init() {
	clientgoscheme.AddToScheme(s)
}

func GetHypershiftDeployment(namespace string, name string, hostingCluster string, hostingNamespace string, override hydapi.InfraOverride) *hydapi.HypershiftDeployment {
	return &hydapi.HypershiftDeployment{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: hydapi.HypershiftDeploymentSpec{
			HostingCluster:   hostingCluster,
			HostingNamespace: hostingNamespace,
			Override:         override,
		},
	}
}

func GetHypershiftDeploymentReconciler() *HypershiftDeploymentReconciler {
	// Log levels: DebugLevel  DebugLevel
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	return &HypershiftDeploymentReconciler{
		Client: clientfake.NewClientBuilder().WithScheme(s).Build(),
		Log:    ctrl.Log.WithName("controllers").WithName("HypershiftDeploymentReconciler"),
		Scheme: s,
		ctx:    context.TODO(),
	}
}

func TestOidcDiscoveryURL(t *testing.T) {
	cases := []struct {
		name         string
		existObj     crclient.Object
		hyd          *hydapi.HypershiftDeployment
		expectedErr  string
		expectBucket string
		expectRegion string
	}{
		{
			name: "err no hostingCluster",
			existObj: &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      constant.HypershiftBucketSecretName,
					Namespace: "testcluster",
				},
				Data: map[string][]byte{
					"bucket": []byte("bucket1"),
					"region": []byte("region1"),
				},
			},
			hyd:         GetHypershiftDeployment("test", "hyd1", "", "mynamespace", ""),
			expectedErr: constant.HostingClusterMissing,
		},
		{
			name: "get info from secret with specific hosting cluster",
			existObj: &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      constant.HypershiftBucketSecretName,
					Namespace: "testcluster",
				},
				Data: map[string][]byte{
					"bucket": []byte("bucket1"),
					"region": []byte("region1"),
				},
			},
			hyd:          GetHypershiftDeployment("test", "hyd1", "testcluster", "mynamespace", ""),
			expectBucket: "bucket1",
			expectRegion: "region1",
		},
		{
			name: "get info from configmap infra config only",
			existObj: &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      constant.HypershiftBucketSecretName,
					Namespace: "testcluster",
				},
				Data: map[string][]byte{
					"bucket": []byte("bucket1"),
					"region": []byte("region1"),
				},
			},
			hyd:          GetHypershiftDeployment("test", "hyd1", "testcluster", "mynamespace", hydapi.InfraConfigureOnly),
			expectBucket: "bucket1",
			expectRegion: "region1",
		},
		{
			name:        "get info from secret not found",
			hyd:         GetHypershiftDeployment("test", "hyd1", "testcluster", "mynamespace", ""),
			expectedErr: "not found",
		},
		{
			name:        "get info from configmap not found",
			hyd:         GetHypershiftDeployment("test", "hyd1", "testcluster", "mynamespace", ""),
			expectedErr: "not found",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			r := GetHypershiftDeploymentReconciler()

			if c.existObj != nil {
				assert.Nil(t, r.Client.Create(ctx, c.existObj, &crclient.CreateOptions{}), "")
			}

			bucket, region, err := oidcDiscoveryURL(r, c.hyd)
			if len(c.expectedErr) == 0 {
				assert.Nil(t, err, "oidc discovery url was successful")
				assert.Equal(t, c.expectBucket, bucket, "bucket equal")
				assert.Equal(t, c.expectRegion, region, "region equal")
			} else {
				assert.Contains(t, err.Error(), c.expectedErr)
			}
		})
	}
}

func getS3Secret(namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      constant.HypershiftBucketSecretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"bucket": []byte("bucket1"),
			"region": []byte("region1"),
		},
	}
}

func TestCreateAwsInfra(t *testing.T) {
	ctx := context.Background()
	hyd := getHDforManifestWork()
	hyd.Spec.HostingCluster = "local-cluster"
	hyd.Spec.Infrastructure.Platform.AWS.Region = ""

	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	corev1.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	r.Client.Create(ctx, getS3Secret("local-cluster"))
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandler{}

	t.Log("Test AwsInfraCreator failure when missing: Spec.Infrastructure.Platform.AWS.Region")
	_, err := r.createAWSInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when conditions are written correctly")
	assert.True(t, meta.IsStatusConditionFalse(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured)),
		"true, when no region is provided and status condition is false")

	t.Log("Test AwsInfraCreator success when region is added")
	hyd.Spec.Infrastructure.Platform.AWS.Region = "us-east-1"
	meta.RemoveStatusCondition(&hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))

	_, err = r.createAWSInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when no problem occurs")
	c := meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionTrue, c.Status, "true, when region is provided")

	t.Log("Test AwsInfraCreator infrastructure function failure")
	r.InfraHandler = &FakeInfraHandlerFailure{}
	hyd.Spec.Infrastructure.Platform.AWS.Region = "us-east-1"
	meta.RemoveStatusCondition(&hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))

	_, err = r.createAWSInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when conditions are written correctly")
	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, metav1.ConditionFalse, c.Status,
		"false, when the AwsInfraCreator fails")
	assert.Equal(t, "failed to create aws infrastructure", c.Message, "error message returned from AwsInfraCreator")
}

func TestDestroyAwsInfra(t *testing.T) {
	ctx := context.Background()
	hyd := getHDforManifestWork()

	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandler{}

	// Test missing: Spec.Infrastructure.Platform.AWS.Region
	_, err := r.destroyAWSInfrastructure(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when destroy is successful")
	c := meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, c.Status, metav1.ConditionFalse, "false, when deleting infrastructure")
	assert.Equal(t, c.Reason, hypdeployment.PlatfromDestroyReason, "reason is Destroying")

	c = meta.FindStatusCondition(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured))
	assert.NotNil(t, c, "not nil, when condition is found")
	assert.Equal(t, c.Status, metav1.ConditionFalse, "false, when deleting iam infrastructure")
	assert.Equal(t, c.Reason, hypdeployment.RemovingReason, "reason is Removing")

	r.InfraHandler = &FakeInfraHandlerFailure{}

	// Test missing: Spec.Infrastructure.Platform.AWS.Region
	_, err = r.destroyAWSInfrastructure(hyd, getProviderSecret())
	assert.NotNil(t, err, "not nil, when condition is set successfully")
	assert.Equal(t, "failed to destroy aws infrastructure", err.Error(), "error from DestroyAWSInfrastructure")
}

func TestCreateAwsInfraIAMMisConfigured(t *testing.T) {
	ctx := context.Background()
	hyd := getHDforManifestWork()
	hyd.Spec.HostingCluster = "local-cluster"
	hyd.Spec.Infrastructure.Platform.AWS.Region = "us-east-1"
	r := GetHypershiftDeploymentReconciler()

	hydapi.AddToScheme(r.Scheme)
	corev1.AddToScheme(r.Scheme)
	r.Client.Create(ctx, hyd)
	r.Client.Create(ctx, getS3Secret("local-cluster"))
	defer r.Client.Delete(ctx, hyd)

	r.InfraHandler = &FakeInfraHandler{}

	// Test missing: Spec.Infrastructure.Platform.AWS.Region
	_, err := r.createAWSInfra(hyd, getProviderSecret())
	assert.Nil(t, err, "nil, when problem condition is written correctly")

	//Everything is setup correctly
	//Disable Credential ARNS and Type
	hyd.Spec.HostedClusterSpec.Platform.Type = "AWS"
	hyd.Spec.Credentials = &hypdeployment.CredentialARNs{}

	_, err = r.createAWSInfra(hyd, getProviderSecret())
	t.Log(hyd.Status.Conditions)

	assert.Nil(t, err, "nil, when problem condition is written correctly")
	assert.True(t, meta.IsStatusConditionFalse(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)),
		"true, when the IAM condition is mis-configured")
}
