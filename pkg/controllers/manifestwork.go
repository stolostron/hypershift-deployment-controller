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

	w := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			// make sure when deploying 2 hostedclusters with the same name but in different namespaces, the
			// generated manifestworks are unqinue.
			Name:      generateManifestName(hyd),
			Namespace: helper.GetTargetManagedCluster(hyd),
			Annotations: map[string]string{
				CreatedByHypershiftDeployment: fmt.Sprintf("%s%s%s",
					hyd.GetNamespace(),
					constant.NamespaceNameSeperator,
					hyd.GetName()),
			},
		},
		Spec: workv1.ManifestWorkSpec{},
	}

	if hyd.Spec.Override == hypdeployment.InfraOverrideDestroy {
		w.Spec.DeleteOption = &workv1.DeleteOption{PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan}
	}

	return w, nil
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
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "Missing targetManagedCluster for override: MANIFESTWORK", hypdeployment.MisConfiguredReason)
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
	if hyd.Spec.Credentials == nil ||
		hyd.Spec.Credentials.AWS == nil {
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

	m.Spec.Workload.Manifests = payload

	// a placeholder for later use
	noOp := func(in *workv1.ManifestWork, payload []workv1.Manifest) controllerutil.MutateFn {
		return func() error {
			return nil
		}
	}

	if _, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, m, noOp(m, payload)); err != nil {
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
		pullCreds := r.scaffoldPullSecret(hyd, *providerSecret)

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
