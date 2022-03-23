// Copyright Contributors to the Open Cluster Management project.

package autoimport

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mcv1 "open-cluster-management.io/api/cluster/v1"

	hydapi "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

const HYD_NAME = "my-hypershift-deployment"
const HYD_NAMESPACE = "testns"

var s = clientgoscheme.Scheme

func init() {
	clientgoscheme.AddToScheme(s)

	hydapi.AddToScheme(s)

	mcv1.AddToScheme(s)
}

func getRequest() ctrl.Request {
	return getRequestWithNamespaceName(HYD_NAMESPACE, HYD_NAME)
}

func getRequestWithNamespaceName(rNamespace string, rName string) ctrl.Request {
	return ctrl.Request{
		NamespacedName: getNamespaceName(rNamespace, rName),
	}
}

func getNamespaceName(namespace string, name string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}
}

func GetHypershiftDeployment(namespace string, name string, infraID string) *hydapi.HypershiftDeployment {
	return &hydapi.HypershiftDeployment{
		ObjectMeta: v1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{FINALIZER},
		},
		Spec: hydapi.HypershiftDeploymentSpec{
			InfraID: infraID,
		},
	}
}

func GetManagedCluster(name string) *mcv1.ManagedCluster {
	return &mcv1.ManagedCluster{
		ObjectMeta: v1.ObjectMeta{
			Name: name,
		},
		Spec: mcv1.ManagedClusterSpec{
			HubAcceptsClient: true,
		},
	}
}

func setAnnotationHYD(hyd *hydapi.HypershiftDeployment, annotations map[string]string) *hydapi.HypershiftDeployment {
	hyd.SetAnnotations(annotations)
	return hyd
}

func setFinalizerHYD(hyd *hydapi.HypershiftDeployment, finalizer []string) *hydapi.HypershiftDeployment {
	hyd.SetFinalizers(finalizer)
	return hyd
}

func setDeletionTimestamp(hyd *hydapi.HypershiftDeployment, deletionTimestamp time.Time) *hydapi.HypershiftDeployment {
	hyd.SetDeletionTimestamp(&v1.Time{Time: deletionTimestamp})
	return hyd
}

func GetHostedClusterKubeconfig(namespace string, name string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"kubeconfig": []byte("test"),
		},
	}
}

func GetAutoImportReconciler() *Reconciler {
	// Log levels: DebugLevel  DebugLevel
	ctrl.SetLogger(zap.New(zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	return &Reconciler{
		Client: clientfake.NewClientBuilder().WithScheme(s).Build(),
		Log:    ctrl.Log.WithName("controllers").WithName("AutoImportReconciler"),
		Scheme: s,
	}
}

func assertAnnoCreateMCFalse(t *testing.T, ctx context.Context, client crclient.Client) {
	var hyd hydapi.HypershiftDeployment
	err := client.Get(ctx, getNamespaceName(HYD_NAMESPACE, HYD_NAME), &hyd)
	assert.Nil(t, err, "hypershift deployment resource is retrieved")
	assert.Contains(t, hyd.Annotations, createManagedClusterAnnotation, "annotation should contain create cm")
	assert.Equal(t, "false", hyd.Annotations[createManagedClusterAnnotation], "assert create cm annotation value be false")
}

func assertAnnoNotContainCreateMC(t *testing.T, ctx context.Context, client crclient.Client) {
	var hyd hydapi.HypershiftDeployment
	err := client.Get(ctx, getNamespaceName(HYD_NAMESPACE, HYD_NAME), &hyd)
	assert.Nil(t, err, "hypershift deployment resource is retrieved")
	assert.NotContains(t, hyd.Annotations, createManagedClusterAnnotation, "annotation should not contain create cm")
}

func assertContainFinalizer(t *testing.T, ctx context.Context, client crclient.Client) {
	var hyd hydapi.HypershiftDeployment
	err := client.Get(ctx, getNamespaceName(HYD_NAMESPACE, HYD_NAME), &hyd)
	assert.Nil(t, err, "hypershift deployment resource is retrieved")
	assert.Contains(t, hyd.Finalizers, FINALIZER, "finalizer should be contained")
}

func TestReconcileCreate(t *testing.T) {
	hyd := GetHypershiftDeployment(HYD_NAMESPACE, HYD_NAME, "id1")

	cases := []struct {
		name            string
		kubesecret      *corev1.Secret
		hyd             *hydapi.HypershiftDeployment
		validateActions func(t *testing.T, ctx context.Context, client crclient.Client)
		expectedErr     string
	}{
		{
			name:       "create managed cluster",
			kubesecret: nil,
			hyd:        hyd.DeepCopy(),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				mcName := helper.ManagedClusterName(hyd)
				err := client.Get(ctx, getNamespaceName("", mcName), &mc)
				assert.Nil(t, err, "when managedCluster resource is retrieved")

				assert.Equal(t, fmt.Sprintf("%s.%s.HypershiftDeployment.cluster.open-cluster-management.io",
					hyd.Name, hyd.Namespace), mc.Annotations[provisionerAnnotation], "assert provisioner annotation value")
				assert.Equal(t, fmt.Sprintf("%s%s%s", hyd.Namespace, constant.NamespaceNameSeperator, hyd.Name),
					mc.Annotations[constant.AnnoHypershiftDeployment], "assert hypershift deployment annotation value")
				assert.Equal(t, "Hosted",
					mc.Annotations["import.open-cluster-management.io/klusterlet-deploy-mode"], "assert hosted mode annotation value")
				assert.Equal(t, helper.GetTargetManagedCluster(hyd),
					mc.Annotations["import.open-cluster-management.io/management-cluster-name"], "assert management cluster annotation value")

				assertAnnoNotContainCreateMC(t, ctx, client)
			},
		},
		{
			name:       "create managed cluster and secret",
			kubesecret: GetHostedClusterKubeconfig(HYD_NAMESPACE, helper.HostedKubeconfigName(hyd)),
			hyd:        hyd.DeepCopy(),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				err := client.Get(ctx, getNamespaceName("", helper.ManagedClusterName(hyd)), &mc)
				assert.Nil(t, err, "managedCluster resource is retrieved")

				var autoImportSecret corev1.Secret
				err = client.Get(ctx, getNamespaceName(mc.Name, "auto-import-secret"), &autoImportSecret)
				assert.Nil(t, err, "managedCluster resource is retrieved")

				assertAnnoCreateMCFalse(t, ctx, client)
			},
		},
		{
			name:       "should not create managed cluster, annotation false",
			kubesecret: GetHostedClusterKubeconfig(HYD_NAMESPACE, helper.HostedKubeconfigName(hyd)),
			hyd:        setAnnotationHYD(hyd.DeepCopy(), map[string]string{createManagedClusterAnnotation: "false"}),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				err := client.Get(ctx, getNamespaceName("", helper.ManagedClusterName(hyd)), &mc)
				assert.True(t, k8serrors.IsNotFound(err), "managed cluster not found")
			},
		},
		{
			name:       "should not create managed cluster, just add finalizer",
			kubesecret: GetHostedClusterKubeconfig(HYD_NAMESPACE, helper.HostedKubeconfigName(hyd)),
			hyd:        setFinalizerHYD(hyd.DeepCopy(), []string{}),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				err := client.Get(ctx, getNamespaceName("", helper.ManagedClusterName(hyd)), &mc)
				assert.Nil(t, err, "managedCluster resource is retrieved")

				assertContainFinalizer(t, ctx, client)
				assertAnnoNotContainCreateMC(t, ctx, client)
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			air := GetAutoImportReconciler()

			if c.hyd != nil {
				assert.Nil(t, air.Client.Create(ctx, c.hyd, &crclient.CreateOptions{}), "")
			}
			if c.kubesecret != nil {
				assert.Nil(t, air.Client.Create(ctx, c.kubesecret, &crclient.CreateOptions{}), "")
			}

			_, err := air.Reconcile(ctx, getRequest())
			assert.Nil(t, err, "reconcile was successful")
			c.validateActions(t, ctx, air.Client)
		})
	}
}

func TestReconcileDelete(t *testing.T) {
	hyd := GetHypershiftDeployment(HYD_NAMESPACE, HYD_NAME, "id1")

	cases := []struct {
		name            string
		managedcluster  *mcv1.ManagedCluster
		hyd             *hydapi.HypershiftDeployment
		validateActions func(t *testing.T, ctx context.Context, client crclient.Client)
		expectedErr     string
	}{
		{
			name:           "delete managed cluster",
			managedcluster: GetManagedCluster(helper.ManagedClusterName(hyd)),
			hyd:            setDeletionTimestamp(hyd.DeepCopy(), time.Now()),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				mcName := helper.ManagedClusterName(hyd)
				err := client.Get(ctx, getNamespaceName("", mcName), &mc)
				assert.True(t, k8serrors.IsNotFound(err), "no managed cluster found")
			},
		},
		{
			name:           "delete managed cluster, no managed cluster created",
			managedcluster: nil,
			hyd:            setDeletionTimestamp(hyd.DeepCopy(), time.Now()),
			validateActions: func(t *testing.T, ctx context.Context, client crclient.Client) {
				var mc mcv1.ManagedCluster
				mcName := helper.ManagedClusterName(hyd)
				err := client.Get(ctx, getNamespaceName("", mcName), &mc)
				assert.True(t, k8serrors.IsNotFound(err), "no managed cluster found")
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx := context.Background()
			air := GetAutoImportReconciler()

			if c.managedcluster != nil {
				assert.Nil(t, air.Client.Create(ctx, c.managedcluster, &crclient.CreateOptions{}), "")
			}

			if c.hyd != nil {
				assert.Nil(t, air.Client.Create(ctx, c.hyd, &crclient.CreateOptions{}), "")
			}

			_, err := air.Reconcile(ctx, getRequest())
			assert.Nil(t, err, "reconcile was successful")
			c.validateActions(t, ctx, air.Client)
		})
	}
}
