/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/util"
	workv1 "open-cluster-management.io/api/work/v1"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

// HypershiftDeploymentReconciler reconciles a HypershiftDeployment object
type HypershiftDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	ctx    context.Context
	Log    logr.Logger
}

const (
	destroyFinalizer       = "hypershiftdeployment.cluster.open-cluster-management.io/finalizer"
	HostedClusterFinalizer = "hypershift.openshift.io/used-by-hostedcluster"
	AutoInfraLabelName     = "hypershift.openshift.io/auto-created-for-infra"
	InfraLabelName         = "hypershift.openshift.io/infra-id"
)

//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.open-cluster-management.io,resources=hypershiftdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=create;get;list;patch;update;watch;deletecollection
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=hypershift.openshift.io,resources=hostedclusters;nodepools,verbs=create;delete;get;list;patch;update;watch
//+kubebuilder:rbac:groups=work.open-cluster-management.io,resources=manifestworks,verbs=create;delete;get;list;patch;update;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HypershiftDeployment object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *HypershiftDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Log = log.FromContext(ctx)
	log := r.Log
	r.ctx = ctx

	log.Info(fmt.Sprintf("Reconcile: %s", req))
	defer log.Info(fmt.Sprintf("Reconcile: %s Done", req))

	var hyd hypdeployment.HypershiftDeployment
	if err := r.Get(ctx, req.NamespacedName, &hyd); err != nil {
		log.V(2).Info("Resource deleted")
		return ctrl.Result{}, nil
	}

	var providerSecret corev1.Secret
	var err error

	configureInfra := hyd.Spec.Infrastructure.Configure
	if configureInfra ||
		(hyd.Spec.HostedClusterSpec != nil && hyd.Spec.HostedClusterSpec.Platform.Azure != nil) {
		secretName := hyd.Spec.Infrastructure.CloudProvider.Name
		err = r.Client.Get(r.ctx, types.NamespacedName{Namespace: hyd.Namespace, Name: secretName}, &providerSecret)
		if err != nil {
			log.Error(err, "Could not retrieve the provider secret")
			r.updateStatusConditionsOnChange(&hyd, hypdeployment.ProviderSecretConfigured, metav1.ConditionFalse, "The secret "+secretName+" could not be retreived from namespace "+hyd.Namespace, hypdeployment.MisConfiguredReason)
			return ctrl.Result{RequeueAfter: 30 * time.Second, Requeue: true}, nil
		}
		if err := r.updateStatusConditionsOnChange(&hyd, hypdeployment.ProviderSecretConfigured, metav1.ConditionTrue, "Retreived secret "+secretName, string(hypdeployment.AsExpectedReason)); err != nil {
			return ctrl.Result{}, err
		}
	}

	oHyd := *hyd.DeepCopy()

	if hyd.Spec.InfraID == "" {
		hyd.Spec.InfraID = fmt.Sprintf("%s-%s", hyd.GetName(), utilrand.String(5))
		log.Info("Using INFRA-ID: " + hyd.Spec.InfraID)
	}

	if !controllerutil.ContainsFinalizer(&hyd, destroyFinalizer) {
		controllerutil.AddFinalizer(&hyd, destroyFinalizer)

		if err := r.patchHypershiftDeploymentResource(&hyd, &oHyd); err != nil || hyd.Spec.InfraID == "" {
			return ctrl.Result{}, fmt.Errorf("failed to update infra-id: \"%s\" and error: %w", hyd.Spec.InfraID, err)
		}

		oHyd = *hyd.DeepCopy()

		// Update the status.conditions. This only works the first time, so if you fix an issue, it will still be set to PlatformXXXMisConfigured
		setStatusCondition(&hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Configuring platform with infra-id: "+hyd.Spec.InfraID, hypdeployment.BeingConfiguredReason)
		setStatusCondition(&hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "Configuring ManifestWork: "+hyd.Spec.InfraID, hypdeployment.BeingConfiguredReason)
		r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Configuring platform IAM with infra-id: "+hyd.Spec.InfraID, hypdeployment.BeingConfiguredReason)
	}

	// Destroying Platform infrastructure used by the HypershiftDeployment scheduled for deletion
	if hyd.DeletionTimestamp != nil {
		return r.destroyHypershift(&hyd, &providerSecret)
	}

	if configureInfra {
		if hyd.Spec.Infrastructure.Platform == nil {
			return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(&hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform")
		}
		if hyd.Spec.Infrastructure.Platform.AWS != nil {
			if requeue, err := r.createAWSInfra(&hyd, &providerSecret); err != nil {
				return requeue, err
			}
		}
		if hyd.Spec.Infrastructure.Platform.Azure != nil {
			if requeue, err := r.createAzureInfra(&hyd, &providerSecret); err != nil {
				return requeue, err
			}
		}
	}

	// Just build the infrastruction platform, do not deploy HostedCluster and NodePool(s)
	if hyd.Spec.Override == hypdeployment.InfraConfigureOnly {
		log.Info("Completed Infrastructure confiugration, skipping HostedCluster and NodePool(s)")
		return ctrl.Result{}, nil
	}

	// Work on the HostedCluster resource
	var hc hyp.HostedCluster
	err = r.Get(ctx, types.NamespacedName{Namespace: hyd.Spec.HostingNamespace, Name: hyd.Name}, &hc)

	// Apply the HostedCluster if Infrastructure is AsExpected or configureInfra: false (user brings their own)
	if (meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)) &&
		meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))) ||
		!configureInfra {

		// hyd.Spec.TargetNamespace is set by both createManifestwork and ScaffoldHostedCluster,
		// using the helper.GetHostingNamespace function

		// In Azure, the providerSecret is needed for Configure true or false
		if hyd.Spec.Override == hypdeployment.InfraConfigureWithManifest {
			log.Info("Wrap hostedCluster, nodepool and secrets to manifestwork")
			return r.createOrUpdateMainfestwork(ctx, req, hyd.DeepCopy(), &providerSecret)
		}

		if apierrors.IsNotFound(err) {

			hostedCluster := ScaffoldHostedCluster(&hyd)

			if err := r.Create(ctx, hostedCluster); err != nil {
				if apierrors.IsAlreadyExists(err) {
					log.Error(err, "Failed to create HostedCluster resource")
					return ctrl.Result{}, err
				}
				log.Info("HostedCluster created " + hc.Name)

			}
			log.Info("HostedCluster resource created: " + hostedCluster.Name)
		} else {
			if !reflect.DeepEqual(hc.Spec.Autoscaling, hyd.Spec.HostedClusterSpec.Autoscaling) ||
				!reflect.DeepEqual(hc.Spec.Release, hyd.Spec.HostedClusterSpec.Release) ||
				!reflect.DeepEqual(hc.Spec.ControllerAvailabilityPolicy, hyd.Spec.HostedClusterSpec.ControllerAvailabilityPolicy) {
				hc.Spec = *hyd.Spec.HostedClusterSpec
				if err := r.Update(ctx, &hc); err != nil {
					log.Error(err, "Failed to update HostedCluster resource")
					return ctrl.Result{}, err
				}
				log.Info("HostedCluster resource updated: " + hc.Name)
			}
		}

		// We loop through what exists, so that we can delete pools if appropriate
		var nodePools hyp.NodePoolList
		if err := r.List(ctx, &nodePools, client.InNamespace(hyd.Spec.HostingNamespace), client.MatchingLabels{AutoInfraLabelName: hyd.Spec.InfraID}); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Processing " + fmt.Sprint(len(nodePools.Items)) + " NodePools")

		// Create and Update HypershiftDeployment.Spec.NodePools
		for _, np := range hyd.Spec.NodePools {
			noMatch := true
			for _, foundNodePool := range nodePools.Items {
				if np.Name == foundNodePool.Name {
					if !reflect.DeepEqual(foundNodePool.Spec, np.Spec) {
						foundNodePool.Spec = np.Spec
						if err := r.Update(ctx, &foundNodePool); err != nil {
							log.Error(err, "Failed to update NodePool resource")
							return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
						}
						log.Info("NodePool resource updated: " + np.Name)
					}
					noMatch = false
					break
				}
			}
			if noMatch {
				nodePool := ScaffoldNodePool(&hyd, np)

				if err := r.Create(ctx, nodePool); err != nil {
					log.Error(err, "Failed to create NodePool resource")
					return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
				}
				log.Info("NodePool resource created: " + np.Name)
			}
		}

		// Delete a NodePool if it no longer is present in the HypershiftDeployment.Spec.NodePools
		for _, nodePool := range nodePools.Items {
			noMatch := true
			for _, np := range hyd.Spec.NodePools {
				if nodePool.Name == np.Name {
					noMatch = false
				}
			}
			if noMatch {
				if nodePool.DeletionTimestamp == nil {
					if err := r.Delete(ctx, &nodePool); err != nil {
						log.Error(err, "Failed to delete NodePool resource")
						return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
					}
					log.Info("NodePool resource deleted: " + nodePool.Name)
				}
			}

		}
	}
	return ctrl.Result{}, nil
}

func (r *HypershiftDeploymentReconciler) scaffoldPullSecret(hyd *hypdeployment.HypershiftDeployment, providerSecret corev1.Secret) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: helper.GetHostingNamespace(hyd),
			Name:      hyd.Spec.HostedClusterSpec.PullSecret.Name,
			Labels: map[string]string{
				AutoInfraLabelName: hyd.Spec.InfraID,
			},
		},
		Data: map[string][]byte{
			".dockerconfigjson": providerSecret.Data["pullSecret"],
		},
	}
}
func (r *HypershiftDeploymentReconciler) createPullSecret(hyd *hypdeployment.HypershiftDeployment, providerSecret corev1.Secret) error {
	_, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, r.scaffoldPullSecret(hyd, providerSecret), func() error { return nil })
	return err
}

func createOIDCSecrets(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment) error {

	for _, secret := range ScaffoldAWSSecrets(hyd) {
		if err := r.Create(r.ctx, secret); apierrors.IsAlreadyExists(err) {
			if err := r.Update(r.ctx, secret); err != nil {
				return err
			}
		}
	}
	return nil
}

func destroySecrets(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment) error {
	//clean up CLI generated secrets
	return r.DeleteAllOf(r.ctx, &corev1.Secret{}, client.InNamespace(hyd.GetNamespace()), client.MatchingLabels{util.AutoInfraLabelName: hyd.Spec.InfraID})

}

func setStatusCondition(hyd *hypdeployment.HypershiftDeployment, conditionType hypdeployment.ConditionType, status metav1.ConditionStatus, message string, reason string) metav1.Condition {
	if hyd.Status.Conditions == nil {
		hyd.Status.Conditions = []metav1.Condition{}
	}
	condition := metav1.Condition{
		Type:               string(conditionType),
		ObservedGeneration: hyd.Generation,
		Status:             status,
		Message:            message,
		Reason:             reason,
	}
	meta.SetStatusCondition(&hyd.Status.Conditions, condition)
	return condition
}

func (r *HypershiftDeploymentReconciler) updateMissingInfrastructureParameterCondition(hyd *hypdeployment.HypershiftDeployment, message string) error {
	setStatusCondition(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Infrastructure missing information", hypdeployment.MisConfiguredReason)
	return r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, message, hypdeployment.MisConfiguredReason)
}

func (r *HypershiftDeploymentReconciler) updateStatusConditionsOnChange(
	hyd *hypdeployment.HypershiftDeployment,
	conditionType hypdeployment.ConditionType,
	conditionStatus metav1.ConditionStatus,
	message string,
	reason string) error {

	inHyd := hyd.DeepCopy()

	var err error = nil
	sc := meta.FindStatusCondition(hyd.Status.Conditions, string(conditionType))

	var checkFunc func() bool

	switch conditionType {
	case hypdeployment.WorkProgressing, hypdeployment.WorkApplied, hypdeployment.WorkAvailable, hypdeployment.WorkDegraded:
		checkFunc = func() bool { // the manifestwork's obeservedGeneration could be different than the hypershiftDeployment's generation
			return sc == nil || sc.Status != conditionStatus || sc.Reason != reason || sc.Message != message
		}

	default:
		checkFunc = func() bool {
			return sc == nil || sc.ObservedGeneration != hyd.Generation || sc.Status != conditionStatus || sc.Reason != reason || sc.Message != message
		}
	}

	if checkFunc() {
		setStatusCondition(hyd, conditionType, conditionStatus, message, reason)

		// use Patch with merge to minimize the update conflicts
		err = r.Client.Status().Patch(r.ctx, hyd, client.MergeFrom(inHyd))
		if err != nil {
			if apierrors.IsConflict(err) {
				r.Log.Error(err, "Conflict encountered when updating HypershiftDeployment.Status")
			} else {
				r.Log.Error(err, "Failed to update HypershiftDeployment.Status")
			}
		}
	}

	return err
}

func (r *HypershiftDeploymentReconciler) patchHypershiftDeploymentResource(hyd *hypdeployment.HypershiftDeployment, inHyd *hypdeployment.HypershiftDeployment) error {
	err := r.Client.Patch(r.ctx, hyd, client.MergeFrom(inHyd))
	if err != nil {
		if apierrors.IsConflict(err) {
			r.Log.Error(err, "Conflict encountered when patching HypershiftDeployment")
		} else {
			r.Log.Error(err, "Failed to update HypershiftDeployment resource")
		}
	}
	return err
}

func (r *HypershiftDeploymentReconciler) destroyHypershift(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {
	log := r.Log
	ctx := r.ctx

	inHyd := hyd.DeepCopy()

	if hyd.Spec.Override == hypdeployment.InfraConfigureWithManifest {
		log.Info("Removing Manifestwork and wait for hostedclsuter and nodepool to be cleaned up.")
		res, err := r.deleteManifestworkWaitCleanUp(ctx, hyd)

		if stErr := r.Client.Status().Patch(ctx, hyd, client.MergeFrom(inHyd)); stErr != nil {
			r.Log.Error(stErr, "Failed to patch HypershiftDeployment.Status while deleting manifestwork")
		}

		if err != nil {
			return res, fmt.Errorf("failed to delete manifestwork %v", err)
		}

		// wait for the nodepools and hostedcluster in target namespace is deleted(via the work agent)
		if !res.IsZero() {
			return res, nil
		}
	} else if hyd.Spec.Override != hypdeployment.InfraOverrideDestroy {
		// Delete nodepools first
		log.Info("Remove any NodePools")
		for _, np := range hyd.Spec.NodePools {
			var nodePool hyp.NodePool
			if err := r.Get(ctx, types.NamespacedName{Namespace: hyd.Spec.HostingNamespace, Name: np.Name}, &nodePool); err == nil {
				if nodePool.DeletionTimestamp == nil {
					r.Log.Info("Deleting NodePool " + np.Name)
					if err := r.Delete(ctx, &nodePool); err != nil {
						log.Error(err, "Failed to delete NodePool resource")
						return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
					}
				}
				log.Info("Waiting for NodePool " + np.Name + " to be deleted, retry in 10s")
				return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
			} else {
				log.Info("NodePool " + np.Name + " already deleted...")
			}
		}

		// Delete the HostedCluster
		var hc hyp.HostedCluster
		if err := r.Get(ctx, types.NamespacedName{Namespace: hyd.Spec.HostingNamespace, Name: hyd.Name}, &hc); !apierrors.IsNotFound(err) {
			if hc.DeletionTimestamp == nil {
				log.Info("Deleting HostedCluster " + hyd.Name)
				// The delete action can take a while and we don't want to block the reconciler
				go r.spawnDelete(ctx, hc)
			}
			log.Info("Waiting for HostedCluster " + hyd.Name + " to be deleted, retry in 10s")
			return ctrl.Result{RequeueAfter: 10 * time.Second, Requeue: true}, nil
		} else {
			log.Info("HostedCluster " + hyd.Name + " already deleted...")
		}
	}

	if hyd.Spec.Override != hypdeployment.InfraOverrideDestroy &&
		hyd.Spec.Infrastructure.Configure {
		// Infrastructure is the last step
		if hyd.Spec.Infrastructure.Platform.AWS != nil {
			if result, err := r.destroyAWSInfrastructure(hyd, providerSecret); err != nil {
				return result, nil // destroyAWSInfrastructure uses requeue times, switch to nil
			}
		}
		if hyd.Spec.Infrastructure.Platform.Azure != nil {
			if result, err := r.destroyAzureInfrastructure(hyd, providerSecret); err != nil {
				return result, nil // destroyAzureInfrastructure uses requeue times, switch to nil
			}
		}
	}

	log.Info("Removing finalizer")
	controllerutil.RemoveFinalizer(hyd, destroyFinalizer)

	if err := r.Client.Update(ctx, hyd); err != nil {
		//if apierrors.IsConflict(err) {
		//	return ctrl.Result{Requeue: true}, nil
		//}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HypershiftDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&hypdeployment.HypershiftDeployment{}).
		Watches(&source.Kind{Type: &workv1.ManifestWork{}},
			handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
				an := obj.GetAnnotations()

				if len(an) == 0 || len(an[CreatedByHypershiftDeployment]) == 0 {
					return []reconcile.Request{}
				}

				res := strings.Split(an[CreatedByHypershiftDeployment], constant.NamespaceNameSeperator)

				if len(res) != 2 {
					r.Log.Error(fmt.Errorf("failed to get manifestwork's hypershiftDeployment"), "")
					return []reconcile.Request{}
				}

				req := reconcile.Request{
					NamespacedName: types.NamespacedName{Namespace: res[0], Name: res[1]},
				}

				return []reconcile.Request{req}
			})).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func (r *HypershiftDeploymentReconciler) spawnDelete(ctx context.Context, hc hyp.HostedCluster) {
	if err := r.Delete(ctx, &hc); err != nil {
		r.Log.Error(err, "Failed to delete "+hc.Kind+" resource")
	} else {
		r.Log.Info("Resource " + hc.Kind + " deleted")
	}
}
