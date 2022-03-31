// Copyright Contributors to the Open Cluster Management project.

package autoimport

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	mcv1 "open-cluster-management.io/api/cluster/v1"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

const DEBUG = 1
const INFO = 0
const WARN = -1
const ERROR = -2
const FINALIZER = "hypershiftdeployment.cluster.open-cluster-management.io/managedcluster-cleanup"
const createManagedClusterAnnotation = "cluster.open-cluster-management.io/createmanagedcluster"
const provisionerAnnotation = "cluster.open-cluster-management.io/provisioner"

// Reconciler reconciles a HypershiftDeployment object to
// import the related hypershift hosted cluster to the hub cluster.
type Reconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=create;get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclusters,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=managedclustersets/join,verbs=create
//+kubebuilder:rbac:groups=register.open-cluster-management.io,resources=managedclusters/accept,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HypershiftDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	log := r.Log.WithValues("AutoImportReconciler", req.NamespacedName)

	var hyd hypdeployment.HypershiftDeployment
	if err := r.Get(ctx, req.NamespacedName, &hyd); err != nil {
		log.V(INFO).Info("Resource hypershift deployment deleted")
		return ctrl.Result{}, nil
	}

	if len(hyd.Spec.InfraID) == 0 {
		log.V(INFO).Info("Resource hypershift spec infraID is empty")
		return ctrl.Result{}, nil
	}

	log.V(INFO).Info("Hypershift Deployment info", "InfraID", hyd.Spec.InfraID,
		"hostingNamespace", hyd.Spec.HostingNamespace, "hostingCluster", hyd.Spec.HostingCluster)

	managedClusterName := helper.ManagedClusterName(&hyd)
	// Delete the ManagedCluster
	if hyd.DeletionTimestamp != nil {
		if err := deleteManagedCluster(r, managedClusterName); err != nil {
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, removeFinalizer(r, &hyd)
	}

	// Do not exit till this point when importmanagedcluster=false, so deletion will work properly if manually imported
	if len(hyd.Annotations) > 0 {
		aValue, found := hyd.Annotations[createManagedClusterAnnotation]
		if found && strings.ToLower(aValue) == "false" {
			log.V(WARN).Info("Skip creation of managedCluster")
			return ctrl.Result{}, nil
		}
	}

	managementClusterName := helper.GetHostingCluster(&hyd)
	// ManagedCluster
	managedCluster, err := ensureManagedCluster(r, req.NamespacedName, managedClusterName, managementClusterName)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Once we are sure there is a ManagedCluster, we set the finalizer
	if !controllerutil.ContainsFinalizer(&hyd, FINALIZER) {
		return ctrl.Result{}, setFinalizer(r, &hyd)
	}

	if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, mcv1.ManagedClusterConditionJoined) {
		// Auto import secret
		var kubeconfig corev1.Secret
		secretNamespaceName := types.NamespacedName{Namespace: managementClusterName, Name: helper.HostedKubeconfigName(&hyd)}
		err = r.Get(ctx, secretNamespaceName, &kubeconfig)
		if k8serrors.IsNotFound(err) {
			log.V(INFO).Info("Wait for the hosted cluster kubeconfig to be created", "secret", secretNamespaceName.String())
			return ctrl.Result{}, nil
		}
		if err := ensureAutoImportSecret(r, managedClusterName, &kubeconfig); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Make sure we don't create the ManagedCluster if it is detached
	return ctrl.Result{}, ensureCreateManagedClusterAnnotationFalse(r, &hyd)
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	syncedFromSpoke := func(object client.Object) bool {
		return strings.EqualFold(object.GetLabels()["synced-from-spoke"], "true")
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&hypdeployment.HypershiftDeployment{}).
		// TODO(zhujian7): After https://github.com/stolostron/hypershift-deployment-controller/pull/33 is merged,
		// we can add a new secret controller to render the kubeConfig in status and remove the watches here.
		Watches(&source.Kind{Type: &corev1.Secret{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			an := obj.GetAnnotations()
			if len(an) == 0 || len(an[constant.AnnoHypershiftDeployment]) == 0 {
				return []reconcile.Request{}
			}

			res := strings.Split(an[constant.AnnoHypershiftDeployment], constant.NamespaceNameSeperator)
			if len(res) != 2 {
				log.Log.Error(fmt.Errorf("failed to get hypershiftDeployment"), "annotation invalid",
					"constant.AnnoHypershiftDeployment", an[constant.AnnoHypershiftDeployment])
				return []reconcile.Request{}
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{Namespace: res[0], Name: res[1]},
			}
			return []reconcile.Request{req}
		}), builder.WithPredicates(
			predicate.Funcs{
				GenericFunc: func(e event.GenericEvent) bool { return false },
				CreateFunc: func(e event.CreateEvent) bool {
					return syncedFromSpoke(e.Object)
				},
				DeleteFunc: func(e event.DeleteEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool {
					if !syncedFromSpoke(e.ObjectNew) {
						return false
					}

					new, okNew := e.ObjectNew.(*corev1.Secret)
					old, okOld := e.ObjectOld.(*corev1.Secret)
					if okNew && okOld {
						return !equality.Semantic.DeepEqual(old.Data, new.Data)
					}
					return false
				},
			},
		)).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1, // This is the default
		}).Named("hypershiftimport").Complete(r)
}

func ensureManagedCluster(r *Reconciler, hydNamespaceName types.NamespacedName,
	managedClusterName, managementClusterName string) (*mcv1.ManagedCluster, error) {
	log := r.Log.WithValues("managedClusterName", managedClusterName)
	ctx := context.Background()

	var mc mcv1.ManagedCluster
	err := r.Get(ctx, types.NamespacedName{Name: managedClusterName}, &mc)
	if k8serrors.IsNotFound(err) {
		log.V(INFO).Info("Create a new ManagedCluster resource")
		mc.Name = managedClusterName
		mc.Spec.HubAcceptsClient = true

		mc.ObjectMeta.Annotations = map[string]string{
			"import.open-cluster-management.io/klusterlet-deploy-mode":  "Hosted",
			"import.open-cluster-management.io/management-cluster-name": managementClusterName,
			constant.AnnoHypershiftDeployment: fmt.Sprintf("%s%s%s",
				hydNamespaceName.Namespace, constant.NamespaceNameSeperator, hydNamespaceName.Name),
			// format is <name>.<namespace>.<kind>.<apiversion>
			// klusterlet addon controller will use this annotation to create klusterletaddonconfig for the hypershift clusters.
			provisionerAnnotation: fmt.Sprintf("%s.%s.HypershiftDeployment.cluster.open-cluster-management.io",
				hydNamespaceName.Name, hydNamespaceName.Namespace),
		}

		if err = r.Create(ctx, &mc, &client.CreateOptions{}); err != nil {
			log.V(ERROR).Info("Could not create ManagedCluster resource", "error", err)
			return nil, err
		}

		return &mc, nil
	}

	if err != nil {
		log.V(WARN).Info("Error when attempting to retreive the ManagedCluster resource", "error", err)
		return nil, err
	}

	return &mc, nil
}

func ensureAutoImportSecret(r *Reconciler, managedClusterName string, kubeSecret *corev1.Secret) error {
	log := r.Log.WithValues("managedClusterName", managedClusterName)
	ctx := context.Background()

	var secret corev1.Secret
	/* #nosec */
	secretName := "auto-import-secret"
	err := r.Get(ctx, types.NamespacedName{Namespace: managedClusterName, Name: secretName}, &secret)
	if k8serrors.IsNotFound(err) {

		log.V(INFO).Info("Create a new auto import secret resource")
		secret.Name = secretName
		secret.Namespace = managedClusterName
		secret.Data = kubeSecret.Data

		if err := r.Create(ctx, &secret, &client.CreateOptions{}); err != nil {
			log.V(ERROR).Info("Could not create auto import secret", "error", err)
			return err
		}
		return nil
	}
	if err != nil {
		log.V(WARN).Info("Error when attempting to retreive the auto import secret", "error", err)
		return err
	}

	return nil
}

func ensureCreateManagedClusterAnnotationFalse(r *Reconciler, hyd *hypdeployment.HypershiftDeployment) error {
	if createmc, ok := hyd.Annotations[createManagedClusterAnnotation]; ok && createmc == "false" {
		return nil
	}

	patch := client.MergeFrom(hyd.DeepCopy())
	if hyd.Annotations == nil {
		hyd.Annotations = make(map[string]string)
	}

	hyd.Annotations[createManagedClusterAnnotation] = "false"
	return r.Client.Patch(context.TODO(), hyd, patch)
}

func setFinalizer(r *Reconciler, hyd *hypdeployment.HypershiftDeployment) error {
	patch := client.MergeFrom(hyd.DeepCopy())
	controllerutil.AddFinalizer(hyd, FINALIZER)
	r.Log.V(INFO).Info("Added finalizer on hypershift deployment: " + hyd.Name)
	return r.Client.Patch(context.TODO(), hyd, patch)
}

func removeFinalizer(r *Reconciler, hyd *hypdeployment.HypershiftDeployment) error {
	if !controllerutil.ContainsFinalizer(hyd, FINALIZER) {
		return nil
	}

	patch := client.MergeFrom(hyd.DeepCopy())
	controllerutil.RemoveFinalizer(hyd, FINALIZER)
	r.Log.V(INFO).Info("Removed finalizer on hypershift deployment: " + hyd.Name)
	return r.Client.Patch(context.TODO(), hyd, patch)
}

func deleteManagedCluster(r *Reconciler, name string) error {
	ctx := context.Background()
	log := r.Log.WithValues("managedClusterName", name)

	var mc mcv1.ManagedCluster
	err := r.Get(ctx, types.NamespacedName{Name: name}, &mc)
	if k8serrors.IsNotFound(err) {
		log.V(INFO).Info("The ManagedCluster resource was not found, skipped")
		return nil
	}
	if err != nil {
		log.V(WARN).Info("Error when attempting to retreive the ManagedCluster resource", "error", err)
		return err
	}

	if mc.DeletionTimestamp != nil {
		log.V(INFO).Info("The managedCluster resource is already being deleted")
		return nil
	}

	err = r.Delete(ctx, &mc)
	if err != nil {
		log.V(WARN).Info("Error while deleting ManagedCluster resource", "error", err)
	}

	log.V(INFO).Info("Deleted ManagedCluster resource")
	return nil
}
