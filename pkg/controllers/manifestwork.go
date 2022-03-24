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
	"errors"
	"fmt"
	"time"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

const (
	ManifestTargetNamespace       = "manifestwork-target-namespace"
	CreatedByHypershiftDeployment = "hypershift-deployment.open-cluster-management.io/created-by"
)

//loadManifest will get hostedclsuter's crs and put them to the manifest array
type loadManifest func(*hypdeployment.HypershiftDeployment, *[]workv1.Manifest) error

func generateManifestName(hyd *hypdeployment.HypershiftDeployment) string {
	return hyd.Spec.InfraID
}

func ScaffoldManifestwork(hyd *hypdeployment.HypershiftDeployment) (*workv1.ManifestWork, error) {

	// TODO @jnpacker, check for the managedCluster as well, or where we validate ClusterSet
	if len(hyd.Spec.InfraID) == 0 {
		return nil, fmt.Errorf("hypershiftDeployment.Spec.InfraID is not set or rendered")
	}

	targetNamespace := helper.GetTargetNamespace(hyd)
	w := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			// make sure when deploying 2 hostedclusters with the same name but in different namespaces, the
			// generated manifestworks are unique.
			Name:      generateManifestName(hyd),
			Namespace: helper.GetTargetManagedCluster(hyd),
			Annotations: map[string]string{
				CreatedByHypershiftDeployment: fmt.Sprintf("%s%s%s",
					hyd.GetNamespace(),
					constant.NamespaceNameSeperator,
					hyd.GetName()),
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{
					{
						RawExtension: runtime.RawExtension{Object: &corev1.Namespace{
							ObjectMeta: metav1.ObjectMeta{
								Name: targetNamespace,
							},
							TypeMeta: metav1.TypeMeta{
								Kind:       "Namespace",
								APIVersion: corev1.SchemeGroupVersion.String(),
							},
						}},
					},
				},
			},
			DeleteOption: &workv1.DeleteOption{
				// Set the delete option to orphan to prevent the manifestwork from being deleted by mistake
				// When we really want to delete the manifestwork, we need to invoke the
				// "setManifestWorkSelectivelyDeleteOption" func to set the delete option before deleting.
				PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
			},
		},
	}

	return w, nil
}

func setManifestWorkSelectivelyDeleteOption(mw *workv1.ManifestWork, targetNamespace string) {
	mw.Spec.DeleteOption = &workv1.DeleteOption{
		PropagationPolicy: workv1.DeletePropagationPolicyTypeSelectivelyOrphan,
		SelectivelyOrphan: &workv1.SelectivelyOrphan{
			OrphaningRules: []workv1.OrphaningRule{
				{
					Resource: "namespaces",
					Name:     targetNamespace,
				},
			},
		},
	}
}

func getManifestWorkKey(hyd *hypdeployment.HypershiftDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      hyd.GetName(),
		Namespace: helper.GetTargetManagedCluster(hyd),
	}
}

func syncManifestworkStatusToHypershiftDeployment(
	hyd *hypdeployment.HypershiftDeployment,
	work *workv1.ManifestWork) {
	workConds := work.Status.Conditions

	for _, cond := range workConds {
		setStatusCondition(
			hyd,
			hypdeployment.ConditionType(cond.Type),
			cond.Status,
			cond.Message,
			cond.Reason,
		)
	}
}

func (r *HypershiftDeploymentReconciler) createMainfestwork(ctx context.Context, req ctrl.Request, hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {

	// We need a targetManagedCluster if we use ManifestWork
	if len(hyd.Spec.TargetManagedCluster) == 0 {
		r.Log.Error(errors.New("targetManagedCluster is empty"), "Spec.targetManagedCluster needs a ManagedCluster name")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "Missing targetManagedCluster with override: MANIFESTWORK", hypdeployment.MisConfiguredReason)
	}

	// Check that a valid spec is present and update the hypershiftDeployment.status.conditions
	// Since you can omit the nodePool, we only check hostedClusterSpec
	if hyd.Spec.HostedClusterSpec == nil {
		r.Log.Error(errors.New("missing value = nil"), "hypershiftDeployment.Spec.HostedClusterSpec is nil")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "HostedClusterSpec is missing", hypdeployment.MisConfiguredReason)
	}

	m, err := ScaffoldManifestwork(hyd)
	if err != nil {
		return ctrl.Result{}, err
	}

	// This is a special check to make sure these values are provided as they are Not part of the standard
	// HostedClusterSpec
	if hyd.Spec.HostedClusterSpec.Platform.AWS != nil &&
		(hyd.Spec.Credentials == nil || hyd.Spec.Credentials.AWS == nil) {
		r.Log.Error(errors.New("hyd.Spec.Credentials.AWS == nil"), "missing IAM configuration")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Missing Spec.Crednetials.AWS.* platform IAM", hypdeployment.MisConfiguredReason)
	}

	// if the manifestwork is created, then move the status to hypershiftDeployment
	// TODO: @ianzhang366 might want to do some upate/patch when the manifestwork is created.
	if err := r.Get(ctx, getManifestWorkKey(hyd), m); err == nil {
		inHyd := hyd.DeepCopy()
		syncManifestworkStatusToHypershiftDeployment(hyd, m)

		return ctrl.Result{},
			r.Client.Status().Patch(r.ctx, hyd, client.MergeFrom(inHyd))
	}

	payload := []workv1.Manifest{}

	manifestFuncs := []loadManifest{
		appendHostedCluster,
		appendNodePool,
		r.appendHostedClusterReferenceSecrets(ctx, providerSecret),
		r.ensureConfiguration(ctx),
	}

	for _, f := range manifestFuncs {
		err := f(hyd, &payload)
		if err != nil {
			r.Log.Error(err, "failed to load paylaod to manifestwork")
			return ctrl.Result{}, err
		}
	}

	// the in object will get override by a GET
	// after the GET, the update will be called and the payload will be wrote to
	// the in object, which will be send with a UPDATE
	update := func(in *workv1.ManifestWork, payload []workv1.Manifest) controllerutil.MutateFn {
		return func() error {
			m.Spec.Workload.Manifests = payload
			return nil
		}
	}
	if _, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, m, update(m, payload)); err != nil {
		r.Log.Error(err, fmt.Sprintf("failed to CreateOrUpdate the existing manifestwork %s", getManifestWorkKey(hyd)))
		return ctrl.Result{}, err

	}

	r.Log.Info(fmt.Sprintf("CreateOrUpdate manifestwork for hypershiftDeployment: %s at targetNamespace: %s", req, helper.GetTargetManagedCluster(hyd)))

	return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
}

func (r *HypershiftDeploymentReconciler) deleteManifestworkWaitCleanUp(ctx context.Context, hyd *hypdeployment.HypershiftDeployment) (ctrl.Result, error) {
	m, err := ScaffoldManifestwork(hyd)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Get(ctx, types.NamespacedName{Name: m.GetName(), Namespace: m.GetNamespace()}, m); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to delete manifestwork, err: %v", err)
	}

	if m.GetDeletionTimestamp().IsZero() {
		patch := client.MergeFrom(m.DeepCopy())
		setManifestWorkSelectivelyDeleteOption(m, helper.GetTargetNamespace(hyd))
		if err := r.Client.Patch(ctx, m, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete manifestwork, set selectively delete option err: %v", err)
		}
		r.Log.Info("pre delete the manifestwork, selectively delete option setting complete")

		if err := r.Delete(ctx, m); err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, fmt.Errorf("failed to delete manifestwork, err: %v", err)
			}
		}
	}

	syncManifestworkStatusToHypershiftDeployment(hyd, m)
	setStatusCondition(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Removing HypershiftDeployment's manifestwork and related resources", hypdeployment.RemovingReason)

	return ctrl.Result{RequeueAfter: 20 * time.Second, Requeue: true}, nil
}

func (r *HypershiftDeploymentReconciler) appendHostedClusterReferenceSecrets(ctx context.Context, providerSecret *corev1.Secret) loadManifest {
	return func(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
		var pullCreds *corev1.Secret
		var err error
		if hyd.Spec.Infrastructure.Configure == false {
			pullCreds, err = r.generateSecret(ctx,
				types.NamespacedName{Name: hyd.Spec.HostedClusterSpec.PullSecret.Name,
					Namespace: hyd.GetNamespace()})

			if err != nil {
				return fmt.Errorf("failed to duplicateSecret, err %w", err)
			}
		} else {
			pullCreds = r.scaffoldPullSecret(hyd, *providerSecret)
		}

		refSecrets := []*corev1.Secret{pullCreds}
		if hyd.Spec.HostedClusterSpec.Platform.AWS != nil {
			refSecrets = append(refSecrets, ScaffoldSecrets(hyd)...)
		} else if hyd.Spec.HostedClusterSpec.Platform.Azure != nil {
			creds, err := getAzureCloudProviderCreds(providerSecret)
			if err != nil {
				return nil
			}
			refSecrets = append(refSecrets, ScaffoldAzureCloudCredential(hyd, creds))
		}

		sshKey := hyd.Spec.HostedClusterSpec.SSHKey
		if len(sshKey.Name) != 0 {
			s, err := r.generateSecret(ctx,
				types.NamespacedName{Name: sshKey.Name,
					Namespace: hyd.GetNamespace()})

			if err != nil {
				return fmt.Errorf("failed to duplicateSecret, err %w", err)
			}

			refSecrets = append(refSecrets, s)
		}

		for _, s := range refSecrets {
			o := duplicateSecretWithOverride(s, overrideNamespace(helper.GetTargetNamespace(hyd)))
			*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: o}})
		}

		return nil
	}
}

func appendHostedCluster(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
	hc := ScaffoldHostedCluster(hyd)

	hc.TypeMeta = metav1.TypeMeta{
		Kind:       "HostedCluster",
		APIVersion: hyp.GroupVersion.String(),
	}

	*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: hc}})

	return nil
}

func appendNodePool(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
	for _, hdNp := range hyd.Spec.NodePools {
		np := ScaffoldNodePool(hyd, hdNp)

		np.TypeMeta = metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: hyp.GroupVersion.String(),
		}

		*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: np}})
	}

	return nil
}
