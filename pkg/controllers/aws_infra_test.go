package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	hydapi "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
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
					Name:      hypershiftBucketSecretName,
					Namespace: "testcluster",
				},
				Data: map[string][]byte{
					"bucket": []byte("bucket1"),
					"region": []byte("region1"),
				},
			},
			hyd:         GetHypershiftDeployment("test", "hyd1", "", "mynamespace", ""),
			expectedErr: helper.HostingClusterMissing,
		},
		{
			name: "get info from secret with specific hosting cluster",
			existObj: &corev1.Secret{
				ObjectMeta: v1.ObjectMeta{
					Name:      hypershiftBucketSecretName,
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
					Name:      hypershiftBucketSecretName,
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
