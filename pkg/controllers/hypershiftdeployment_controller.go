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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/go-logr/logr"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	"github.com/openshift/hypershift/cmd/util"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	workv1 "open-cluster-management.io/api/work/v1"
)

// HypershiftDeploymentReconciler reconciles a HypershiftDeployment object
type HypershiftDeploymentReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	ctx    context.Context
	Log    logr.Logger
}

//loadManifest will get hostedclsuter's crs and put them to the manifest array
type loadManifest func(*hypdeployment.HypershiftDeployment, *[]workv1.Manifest)

const (
	destroyFinalizer       = "hypershiftdeployment.cluster.open-cluster-management.io/finalizer"
	HostedClusterFinalizer = "hypershift.openshift.io/used-by-hostedcluster"
	oidcStorageProvider    = "oidc-storage-provider-s3-config"
	oidcSPNamespace        = "kube-public"
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
	if configureInfra {
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
		r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Configuring platform IAM with infra-id: "+hyd.Spec.InfraID, hypdeployment.BeingConfiguredReason)
	}

	// Destroying Platform infrastructure used by the HypershiftDeployment scheduled for deletion
	if hyd.DeletionTimestamp != nil {
		return r.destroyHypershift(&hyd, &providerSecret)
	}

	if configureInfra && hyd.Spec.Infrastructure.Platform == nil {
		return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(&hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform")
	}

	var infraOut *aws.CreateInfraOutput
	var iamOut *aws.CreateIAMOutput

	if configureInfra && hyd.Spec.Infrastructure.Platform.AWS != nil {
		if hyd.Spec.Infrastructure.Platform.AWS.Region == "" {
			return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(&hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform.AWS.Region")
		}

		// Skip reconcile based on condition
		// Does both INFRA and IAM, as IAM depends on zoneID's from INFRA
		if !meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured)) ||
			!meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)) {

			log.Info("Creating infrastructure on the provider that will be used by the HypershiftDeployment, HostedClusters & NodePools")
			o := aws.CreateInfraOptions{
				AWSKey:       string(providerSecret.Data["aws_access_key_id"]),
				AWSSecretKey: string(providerSecret.Data["aws_secret_access_key"]),
				Region:       hyd.Spec.Infrastructure.Platform.AWS.Region,
				InfraID:      hyd.Spec.InfraID,
				Name:         hyd.GetName(),
				BaseDomain:   string(providerSecret.Data["baseDomain"]),
			}

			infraOut, err = o.CreateInfra(r.ctx)
			if err != nil {
				log.Error(err, "Could not create infrastructure")

				return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true},
					r.updateStatusConditionsOnChange(
						&hyd, hypdeployment.PlatformConfigured,
						metav1.ConditionFalse,
						err.Error(),
						hypdeployment.MisConfiguredReason)
			}

			// This creates the required HostedClusterSpec and NodePoolSpec(s), from scratch or if supplied
			ScaffoldHostedClusterSpec(&hyd, infraOut)
			ScaffoldNodePoolSpec(&hyd, infraOut)

			if err := r.patchHypershiftDeploymentResource(&hyd, &oHyd); err != nil {
				r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
				return ctrl.Result{}, err
			}

			if err := r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason); err != nil {
				return ctrl.Result{}, err
			}
			log.Info("Infrastructure configured")

			if err := r.Get(ctx, req.NamespacedName, &hyd); err != nil {
				return ctrl.Result{}, nil
			}

			oHyd = *hyd.DeepCopy()

			oidcSPName, oidcSPRegion, iamErr := oidcDiscoveryURL(r, hyd.Spec.InfraID)
			if iamErr == nil {
				iamOpt := aws.CreateIAMOptions{
					Region:                          hyd.Spec.Infrastructure.Platform.AWS.Region,
					AWSKey:                          string(providerSecret.Data["aws_access_key_id"]),
					AWSSecretKey:                    string(providerSecret.Data["aws_secret_access_key"]),
					InfraID:                         hyd.Spec.InfraID,
					IssuerURL:                       "", //This is generated on the fly by CreateIAMOutput
					AdditionalTags:                  []string{},
					OIDCStorageProviderS3BucketName: oidcSPName,
					OIDCStorageProviderS3Region:     oidcSPRegion,
					PrivateZoneID:                   infraOut.PrivateZoneID,
					PublicZoneID:                    infraOut.PublicZoneID,
					LocalZoneID:                     infraOut.LocalZoneID,
				}

				iamOut, iamErr = iamOpt.CreateIAM(r.ctx, r.Client)
				if iamErr == nil {
					if iamErr = createOIDCSecrets(r, &hyd, iamOut); iamErr == nil {
						if iamErr = r.createPullSecret(&hyd, providerSecret); iamErr == nil {
							hyd.Spec.HostedClusterSpec.IssuerURL = iamOut.IssuerURL
							hyd.Spec.HostedClusterSpec.Platform.AWS.Roles = iamOut.Roles
							if err := r.patchHypershiftDeploymentResource(&hyd, &oHyd); err != nil {
								return ctrl.Result{}, r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
							}
							r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
							log.Info("IAM and Secrets configured")
						}
					}
				}
			}
			if iamErr != nil {
				r.updateStatusConditionsOnChange(&hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, iamErr.Error(), hypdeployment.ConfiguredAsExpectedReason)
				return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true}, iamErr
			}
		}
	}

	// Just build the infrastruction platform, do not deploy HostedCluster and NodePool(s)
	if hyd.Spec.Override == hypdeployment.InfraConfigureOnly {
		log.Info("Completed Infrastructure confiugration, skipping HostedCluster and NodePool(s)")
		return ctrl.Result{}, nil
	}

	if hyd.Spec.Override == hypdeployment.InfraConfigureWithManifest {
		log.Info("Wrap hostedCluster, nodepool and secrets to manifestwork")
		return r.createMainfestwork(ctx, req, hyd.DeepCopy())
	}

	// Work on the HostedCluster resource
	var hc hyp.HostedCluster
	err = r.Get(ctx, types.NamespacedName{Namespace: hyd.Namespace, Name: hyd.Name}, &hc)

	// Apply the HostedCluster if Infrastructure is AsExpected or configureInfra: false (user brings their own)
	if (meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)) &&
		meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))) ||
		!configureInfra {
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
	}

	// Work on the NodePool resources
	// Apply NodePool(s) if Infrastructure is AsExpected or configureInfra: false (user brings their own)
	if (meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)) &&
		meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured))) ||
		!configureInfra {

		// We loop through what exists, so that we can delete pools if appropriate
		var nodePools hyp.NodePoolList
		if err := r.List(ctx, &nodePools, client.MatchingLabels{AutoInfraLabelName: hyd.Spec.InfraID}); err != nil {
			return ctrl.Result{}, err
		}

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

func oidcDiscoveryURL(r *HypershiftDeploymentReconciler, infraID string) (string, string, error) {

	cm := &corev1.ConfigMap{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: oidcStorageProvider, Namespace: oidcSPNamespace}, cm); err != nil {
		return "", "", err
	}
	return cm.Data["name"], cm.Data["region"], nil
}

func (r *HypershiftDeploymentReconciler) createPullSecret(hyd *hypdeployment.HypershiftDeployment, providerSecret corev1.Secret) error {

	buildPullSecret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: hyd.Namespace,
			Name:      hyd.Name + "-pull-secret",
			Labels: map[string]string{
				AutoInfraLabelName: hyd.Spec.InfraID,
			},
		},
		Data: map[string][]byte{
			".dockerconfigjson": providerSecret.Data["pullSecret"],
		},
	}
	if err := r.Create(r.ctx, buildPullSecret); apierrors.IsAlreadyExists(err) {
		if err := r.Update(r.ctx, buildPullSecret); err != nil {
			return err
		}
	}
	return nil
}

func createOIDCSecrets(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment, iamInfo *aws.CreateIAMOutput) error {

	buildAWSCreds := func(name, arn string) *corev1.Secret {
		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: hyd.Namespace,
				Name:      name,
				Labels: map[string]string{
					AutoInfraLabelName: hyd.Spec.InfraID,
				},
			},
			Data: map[string][]byte{
				"credentials": []byte(fmt.Sprintf(`[default]
	role_arn = %s
	web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
	`, arn)),
			},
		}
	}

	secretResource := buildAWSCreds(hyd.Name+"-cpo-creds", iamInfo.ControlPlaneOperatorRoleARN)
	if err := r.Create(r.ctx, secretResource); apierrors.IsAlreadyExists(err) {
		if err := r.Update(r.ctx, secretResource); err != nil {
			return err
		}
	}

	secretResource = buildAWSCreds(hyd.Name+"-cloud-ctrl-creds", iamInfo.KubeCloudControllerRoleARN)
	if err := r.Create(r.ctx, secretResource); apierrors.IsAlreadyExists(err) {
		if err := r.Update(r.ctx, secretResource); err != nil {
			return err
		}
	}

	secretResource = buildAWSCreds(hyd.Name+"-node-mgmt-creds", iamInfo.NodePoolManagementRoleARN)
	if err := r.Create(r.ctx, secretResource); apierrors.IsAlreadyExists(err) {
		if err := r.Update(r.ctx, secretResource); err != nil {
			return err
		}

	}
	return nil
}

func destroyOIDCSecrets(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment) error {
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
		log.Info("Removing Manifestwork and wait for hostedclsuter and noodpool to be cleaned up.")
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
			if err := r.Get(ctx, types.NamespacedName{Namespace: hyd.Namespace, Name: np.Name}, &nodePool); err == nil {
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
		if err := r.Get(ctx, types.NamespacedName{Namespace: hyd.Namespace, Name: hyd.Name}, &hc); !apierrors.IsNotFound(err) {
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

	if hyd.Spec.Override != hypdeployment.InfraOverrideDestroy {
		// Infrastructure is the last step
		dOpts := aws.DestroyInfraOptions{
			AWSCredentialsFile: "",
			AWSKey:             string(providerSecret.Data["aws_access_key_id"]),
			AWSSecretKey:       string(providerSecret.Data["aws_secret_access_key"]),
			Region:             hyd.Spec.Infrastructure.Platform.AWS.Region,
			BaseDomain:         string(providerSecret.Data["baseDomain"]),
			InfraID:            hyd.Spec.InfraID,
			Name:               hyd.GetName(),
		}

		setStatusCondition(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Destroying HypershiftDeployment with infra-id: "+hyd.Spec.InfraID, hypdeployment.PlatfromDestroyReason)
		r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Removing HypershiftDeployment IAM with infra-id: "+hyd.Spec.InfraID, hypdeployment.RemovingReason)

		log.Info("Deleting Infrastructure on provider")
		if err := dOpts.DestroyInfra(ctx); err != nil {
			log.Error(err, "there was a problem destroying infrastructure on the provider, retrying in 30s")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		log.Info("Deleting Infrastructure IAM on provider")
		iamOpt := aws.DestroyIAMOptions{
			Region:       hyd.Spec.Infrastructure.Platform.AWS.Region,
			AWSKey:       dOpts.AWSKey,
			AWSSecretKey: dOpts.AWSSecretKey,
			InfraID:      dOpts.InfraID,
		}

		if err := iamOpt.DestroyIAM(ctx); err != nil {
			log.Error(err, "failed to delete IAM on provider")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		log.Info("Deleting OIDC secrets")
		if err := destroyOIDCSecrets(r, hyd); err != nil {
			log.Error(err, "Encountered an issue while deleting OIDC secrets")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
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

				res := strings.Split(an[CreatedByHypershiftDeployment], NamespaceNameSeperator)

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
