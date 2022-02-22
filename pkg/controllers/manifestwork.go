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
	"time"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ManifestTargetNamespace       = "manifestwork-target-namespace"
	CreatedByHypershiftDeployment = "hypershift-deployment.open-cluster-management.io/created-by"
	NamespaceNameSeperator        = "/"
)

func ScafoldManifestwork(hyd *hypdeployment.HypershiftDeployment) *workv1.ManifestWork {
	w := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hyd.GetName(),
			Namespace: getTargetManagedCluster(hyd),
			Annotations: map[string]string{
				CreatedByHypershiftDeployment: fmt.Sprintf("%s%s%s",
					hyd.GetNamespace(),
					NamespaceNameSeperator,
					hyd.GetName()),
			},
		},
		Spec: workv1.ManifestWorkSpec{},
	}

	if hyd.Spec.Override == hypdeployment.InfraOverrideDestroy {
		w.Spec.DeleteOption = &workv1.DeleteOption{PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan}
	}

	return w
}

func getManifestWorkKey(hyd *hypdeployment.HypershiftDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      hyd.GetName(),
		Namespace: getTargetManagedCluster(hyd),
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

func (r *HypershiftDeploymentReconciler) createMainfestwork(ctx context.Context, req ctrl.Request, hyd *hypdeployment.HypershiftDeployment) (ctrl.Result, error) {
	m := ScafoldManifestwork(hyd)
	// if the manifestwork is created, then move the status to hypershiftDeployment
	// TODO: @ianzhang366 might want to do some upate/patch when the manifestwork is created.
	if err := r.Get(ctx, getManifestWorkKey(hyd), m); err == nil {
		inHyd := hyd.DeepCopy()
		syncManifestworkStatusToHypershiftDeployment(hyd, m)

		return ctrl.Result{},
			r.Client.Status().Patch(r.ctx, hyd, client.MergeFrom(inHyd))

	}

	appendSecrets, err := r.appendReferenceSecrets(ctx, hyd)
	if err != nil {
		return ctrl.Result{}, err
	}
	payload := []workv1.Manifest{}

	manifestFuncs := []loadManifest{
		appendHostedCluster,
		appendNodePool,
		appendSecrets,
	}

	for _, f := range manifestFuncs {
		f(hyd, &payload)
	}

	m.Spec.Workload.Manifests = payload

	if err := r.Create(r.ctx, m); err != nil {
		if apierrors.IsAlreadyExists(err) {
			//TODO: ianzhang366, might want to patch the manifestwork over here.
			return ctrl.Result{}, r.Update(r.ctx, m)
		}

		return ctrl.Result{}, fmt.Errorf("failed to create manifestwork based on hypershiftDeployment: %s, err: %w", req, err)
	}

	r.Log.Info(fmt.Sprintf("created manifestwork for hypershiftDeployment: %s at targetNamespace: %s", req, getTargetManagedCluster(hyd)))

	return ctrl.Result{}, nil
}

func (r *HypershiftDeploymentReconciler) deleteManifestworkWaitCleanUp(ctx context.Context, hyd *hypdeployment.HypershiftDeployment) (ctrl.Result, error) {
	m := ScafoldManifestwork(hyd)

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

func (r *HypershiftDeploymentReconciler) appendReferenceSecrets(ctx context.Context, hyd *hypdeployment.HypershiftDeployment) (loadManifest, error) {
	cpoCreds := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: hyd.Spec.HostedClusterSpec.Platform.AWS.ControlPlaneOperatorCreds.Name,
		Namespace: hyd.GetNamespace()}, cpoCreds); err != nil {
		return nil, fmt.Errorf("failed to get the cpo creds, err: %w", err)
	}

	kccCreds := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: hyd.Spec.HostedClusterSpec.Platform.AWS.KubeCloudControllerCreds.Name,
		Namespace: hyd.GetNamespace()}, kccCreds); err != nil {
		return nil, fmt.Errorf("failed to get the cpo creds, err: %w", err)
	}

	nmcCreds := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: hyd.Spec.HostedClusterSpec.Platform.AWS.NodePoolManagementCreds.Name,
		Namespace: hyd.GetNamespace()}, nmcCreds); err != nil {
		return nil, fmt.Errorf("failed to get the cpo creds, err: %w", err)
	}

	pullCreds := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: hyd.Spec.HostedClusterSpec.PullSecret.Name,
		Namespace: hyd.GetNamespace()}, pullCreds); err != nil {
		return nil, fmt.Errorf("failed to get the cpo creds, err: %w", err)
	}

	overrideSecret := func(in *corev1.Secret) *corev1.Secret {
		out := &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
		}

		out.SetName(in.GetName())
		out.SetNamespace(getTargetNamespace(hyd))
		out.SetLabels(in.GetLabels())
		out.Data = in.Data

		return out
	}

	refSecrets := []*corev1.Secret{cpoCreds, kccCreds, nmcCreds, pullCreds}

	return func(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) {
		for _, s := range refSecrets {
			o := overrideSecret(s)
			*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: o}})
		}

	}, nil
}

//TODO @ianzhang366 integrate with the clusterSet logic
func getTargetManagedCluster(hyd *hypdeployment.HypershiftDeployment) string {
	if len(hyd.Spec.TargetManagedCluster) == 0 {
		return hyd.GetNamespace()
	}

	return hyd.Spec.TargetManagedCluster
}

func appendHostedCluster(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) {
	hc := ScaffoldHostedCluster(hyd)

	hc.TypeMeta = metav1.TypeMeta{
		Kind:       "HostedCluster",
		APIVersion: hyp.GroupVersion.String(),
	}

	*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: hc}})
}

func appendNodePool(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) {
	for _, hdNp := range hyd.Spec.NodePools {
		np := ScaffoldNodePool(hyd, hdNp)

		np.TypeMeta = metav1.TypeMeta{
			Kind:       "NodePool",
			APIVersion: hyp.GroupVersion.String(),
		}

		*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: np}})
	}
}
