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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	hyp "github.com/openshift/hypershift/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	condmeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	workv1 "open-cluster-management.io/api/work/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	hydclient "github.com/stolostron/hypershift-deployment-controller/pkg/client"
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

// variables defined for manifestwork status sync
var (
	HostedClusterResource = "hostedclusters"
	NodePoolResource      = "nodepools"
	Reason                = "reason"
	StatusFlag            = "status"
	Message               = "message"
	Progress              = "progress"
)

//loadManifest will get hostedclsuter's crs and put them to the manifest array
type loadManifest func(*hypdeployment.HypershiftDeployment, *[]workv1.Manifest) error

var mLog = ctrl.Log.WithName("manifestworks")

func generateManifestName(hyd *hypdeployment.HypershiftDeployment) string {
	return hyd.Spec.InfraID
}

func scaffoldManifestwork(hyd *hypdeployment.HypershiftDeployment) (*workv1.ManifestWork, error) {

	// TODO @jnpacker, check for the managedCluster as well, or where we validate ClusterSet
	if len(hyd.Spec.InfraID) == 0 {
		return nil, fmt.Errorf("hypershiftDeployment.Spec.InfraID is not set or rendered")
	}

	k := getManifestWorkKey(hyd)

	w := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			// make sure when deploying 2 hostedclusters with the same name but in different namespaces, the
			// generated manifestworks are unique.
			Name:      k.Name,
			Namespace: k.Namespace,
			Annotations: map[string]string{
				constant.CreatedByHypershiftDeployment: fmt.Sprintf("%s%s%s",
					hyd.GetNamespace(),
					constant.NamespaceNameSeperator,
					hyd.GetName()),
			},
		},
		Spec: workv1.ManifestWorkSpec{
			Workload: workv1.ManifestsTemplate{
				Manifests: []workv1.Manifest{},
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

func setManifestWorkSelectivelyDeleteOption(mw *workv1.ManifestWork, hyd *hypdeployment.HypershiftDeployment) {
	hostingNamespace := helper.GetHostingNamespace(hyd)

	if hyd.Spec.Override == hypdeployment.InfraOverrideDestroy {
		mw.Spec.DeleteOption = &workv1.DeleteOption{
			PropagationPolicy: workv1.DeletePropagationPolicyTypeOrphan,
		}
	} else if hyd.Spec.Override == hypdeployment.DeleteHostingNamespace {
		mw.Spec.DeleteOption = &workv1.DeleteOption{
			PropagationPolicy: workv1.DeletePropagationPolicyTypeForeground,
		}
	} else {
		mw.Spec.DeleteOption = &workv1.DeleteOption{
			PropagationPolicy: workv1.DeletePropagationPolicyTypeSelectivelyOrphan,
			SelectivelyOrphan: &workv1.SelectivelyOrphan{
				OrphaningRules: []workv1.OrphaningRule{
					{
						Resource: "namespaces",
						Name:     hostingNamespace,
					},
				},
			},
		}
	}
}

func getManifestWorkKey(hyd *hypdeployment.HypershiftDeployment) types.NamespacedName {
	return types.NamespacedName{
		Name:      generateManifestName(hyd),
		Namespace: helper.GetHostingCluster(hyd),
	}
}

func syncManifestworkStatusToHypershiftDeployment(
	hyd *hypdeployment.HypershiftDeployment,
	work *workv1.ManifestWork) {
	workConds := work.Status.Conditions

	conds := []metav1.Condition{}

	conds = append(conds, workConds...)

	feedback := getStatusFeedbackAsCondition(work, hyd)
	conds = append(conds, feedback...)

	for _, cond := range conds {
		setStatusCondition(
			hyd,
			hypdeployment.ConditionType(cond.Type),
			cond.Status,
			cond.Message,
			cond.Reason,
		)
	}
}

func (r *HypershiftDeploymentReconciler) validateHostedClusterAndNodePool(ctx context.Context, hcName string, hcSpec hyp.HostedClusterSpec, npSpec hyp.NodePoolSpec) error {
	// Platform.Type in NodePool matches the HostedCluster
	if npSpec.Platform.Type != hcSpec.Platform.Type {
		r.Log.Error(errors.New("Platform.Type value mismatch"), "Platform.Type in node pool(s) does not match value in HostedClusterSpec")
		return errors.New("Platform.Type value mismatch")
	}

	// NodePool references the correct hostedCluster
	if npSpec.ClusterName != hcName {
		r.Log.Error(errors.New("incorrect Spec.ClusterName in NodePool"), "Spec.ClusterName in NodePool needs to match the referenced hostedCluster")
		return errors.New("incorrect Spec.ClusterName in NodePool")
	}

	// Release.Image in NodePool matches the HostedCluster
	if npSpec.Release.Image != hcSpec.Release.Image {
		r.Log.Info("Release.Image in node pool(s) does not match value in HostedClusterSpec")
	}

	return nil
}

// validateSecurityConstraints checks the given HypershiftDeployment has the right permission to work on a given hosting cluster
// return true if all the checks passed or we are skipping validation, return false if any of the check fails
func (r *HypershiftDeploymentReconciler) validateSecurityConstraints(ctx context.Context, hyd *hypdeployment.HypershiftDeployment) (bool, error) {
	if !r.ValidateClusterSecurity {
		r.Log.Info("Skipping validate security constraints.")
		return true, nil
	}

	// Check the namespace being used has valid managed cluster set bindings
	cbg := hydclient.ClusterSetBindingsGetter{
		Client: r.Client,
	}

	bindings, err := clusterv1beta1.GetBoundManagedClusterSetBindings(hyd.Namespace, cbg)
	if err != nil {
		r.Log.Error(err, hyd.Namespace+" namespace needs at least one bound ManagedClusterSetBinding")
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			"a bound ManagedClusterSetBinding is required in namespace "+hyd.Namespace+" retrying again after a minute", hypdeployment.MisConfiguredReason)
	}

	if len(bindings) == 0 {
		r.Log.Error(errors.New("missing a bound ManagedClusterSetBinding in namespace "+hyd.Namespace), hyd.Namespace+" namespace needs at least one bound ManagedClusterSetBinding")
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			"a bound ManagedClusterSetBinding is required in namespace "+hyd.Namespace+" retrying again after a minute", hypdeployment.MisConfiguredReason)
	}

	clusterSets := sets.NewString()

	for _, binding := range bindings {
		clusterSets.Insert(binding.Name)
	}

	// Check the managed cluster exists
	var managedCluster clusterv1.ManagedCluster
	err = r.Get(ctx, types.NamespacedName{Name: hyd.Spec.HostingCluster}, &managedCluster)
	switch {
	case apierrors.IsNotFound(err):
		r.Log.Error(err, "fail to find ManagedCluster: "+hyd.Spec.HostingCluster)
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			hyd.Spec.HostingCluster+" ManagedCluster is required. Retrying after a minute", hypdeployment.MisConfiguredReason)
	case err != nil:
		r.Log.Error(err, "error while trying to find ManagedCluster: "+hyd.Spec.HostingCluster)
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			hyd.Spec.HostingCluster+" ManagedCluster is required. Retrying after a minute", hypdeployment.MisConfiguredReason)
	}

	foundClusterSet, err := helper.IsClusterInClusterSet(r.Client, &managedCluster, clusterSets.List())
	if err != nil {
		r.Log.Error(err, "error while trying to determine if ManagedCluster: "+hyd.Spec.HostingCluster+" is in a ManagedClusterSet")
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			hyd.Spec.HostingCluster+" ManagedClusterSet is required. Retrying after a minute", hypdeployment.MisConfiguredReason)
	}

	if !foundClusterSet {
		r.Log.Error(errors.New("Spec.HostingCluster is not in a ManagedClusterSet"), "Spec.HostingCluster needs to be a member of a ManagedClusterSet")
		return false, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
			"HostingCluster needs to be a ManagedCluster that is a member of a ManagedClusterSet. Retrying after a minute", hypdeployment.MisConfiguredReason)
	}

	return true, nil
}

func (r *HypershiftDeploymentReconciler) createOrUpdateMainfestwork(ctx context.Context, req ctrl.Request, hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {

	// We need a HostingCluster if we use ManifestWork
	if len(hyd.Spec.HostingCluster) == 0 {
		r.Log.Error(errors.New(constant.HostingClusterMissing), "Spec.HostingCluster needs a ManagedCluster name")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, constant.HostingClusterMissing, hypdeployment.MisConfiguredReason)
	}

	// Check that a valid spec, for infra.configure=T, is present and update the hypershiftDeployment.status.conditions
	// Since you can omit the nodePool, we only check hostedClusterSpec
	if hyd.Spec.HostedClusterSpec == nil && hyd.Spec.Infrastructure.Configure {
		r.Log.Error(errors.New("missing value = nil"), "hypershiftDeployment.Spec.HostedClusterSpec is nil")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "HostedClusterSpec is missing", hypdeployment.MisConfiguredReason)
	}

	// For infra.configure=F, either HostedClusterSpec or HostedClusterRef is required
	if !hyd.Spec.Infrastructure.Configure && len(hyd.Spec.HostedClusterRef.Name) == 0 && hyd.Spec.HostedClusterSpec == nil {
		r.Log.Error(errors.New("missing value = nil"), "hypershiftDeployment.Spec.HostedClusterSpec and hypershiftDeployment.Spec.HostedClusterRef are nil")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "HostedClusterSpec or HostedClusterRef is required", hypdeployment.MisConfiguredReason)
	}

	// Check hostedClusterRef and NodePoolRefs exist and their platform.type matches
	if len(hyd.Spec.HostedClusterRef.Name) != 0 && len(hyd.Spec.NodePoolsRef) != 0 {
		// OK to use typed client since it's just for validation
		hc := &hyp.HostedCluster{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: hyd.Namespace, Name: hyd.Spec.HostedClusterRef.Name}, hc); err != nil {
			r.Log.Error(errors.New("hostedCluster not found"), "hostedCluster %v is expected in namespace %v", hyd.Spec.HostedClusterRef.Name, hyd.Namespace)
			return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "hostedCluster not found", hypdeployment.MisConfiguredReason)
		}

		for _, npRef := range hyd.Spec.NodePoolsRef {
			np := &hyp.NodePool{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: hyd.Namespace, Name: npRef.Name}, np); err != nil {
				r.Log.Error(errors.New("nodePool not found"), "nodePool %v is expected in namespace %v", npRef.Name, hyd.Namespace)
				return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "nodePool not found", hypdeployment.MisConfiguredReason)
			}

			if err := r.validateHostedClusterAndNodePool(ctx, hc.Name, hc.Spec, np.Spec); err != nil {
				return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
			}
		}
	}

	// For hostedClusterSpec and nodePoolSpec, check that the platform.type matches
	if hyd.Spec.HostedClusterSpec != nil && len(hyd.Spec.NodePools) != 0 {
		for _, np := range hyd.Spec.NodePools {
			if err := r.validateHostedClusterAndNodePool(ctx, hyd.Name, *hyd.Spec.HostedClusterSpec, np.Spec); err != nil {
				return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
			}
		}
	}

	passedSecurity, statusUpdateErr := r.validateSecurityConstraints(ctx, hyd)
	if !passedSecurity {
		return ctrl.Result{}, statusUpdateErr
	}

	m, err := scaffoldManifestwork(hyd)
	if err != nil {
		return ctrl.Result{RequeueAfter: time.Minute * 1}, err
	}

	mwCfg := enableManifestStatusFeedback(m, hyd)

	// This is a special check to make sure these values are provided as they are Not part of the standard
	// HostedClusterSpec
	if hyd.Spec.HostedClusterSpec != nil && hyd.Spec.HostedClusterSpec.Platform.AWS != nil &&
		(hyd.Spec.Credentials == nil || hyd.Spec.Credentials.AWS == nil) {
		r.Log.Error(errors.New("hyd.Spec.Credentials.AWS == nil"), "missing IAM configuration")
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Missing Spec.Crednetials.AWS.* platform IAM", hypdeployment.MisConfiguredReason)
	}

	inHyd := hyd.DeepCopy()
	// if the manifestwork is created, then move the status to hypershiftDeployment
	if err := r.Get(ctx, getManifestWorkKey(hyd), m); err == nil {
		syncManifestworkStatusToHypershiftDeployment(hyd, m)
	}

	payload := []workv1.Manifest{}

	manifestFuncs := []loadManifest{
		ensureTaregetNamespace,
		r.appendHostedCluster(ctx),
		r.appendNodePool(ctx),
		r.appendHostedClusterReferenceSecrets(ctx, providerSecret),
		r.ensureConfiguration(ctx, m),
	}

	for _, f := range manifestFuncs {
		err := f(hyd, &payload)
		if err != nil {
			r.Log.Error(err, "failed to load payload to manifestwork")
			return ctrl.Result{}, err
		}
	}

	// the object in controllerutil.CreateOrUpdate will get override by a GET
	// after the GET, the update will be called and the payload will be wrote to
	// the in object, which will be send with a UPDATE
	update := func(in *workv1.ManifestWork, payload []workv1.Manifest) controllerutil.MutateFn {
		return func() error {
			m.Spec.Workload.Manifests = payload
			m.Spec.ManifestConfigs = mwCfg
			return nil
		}
	}
	if _, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, m, update(m, payload)); err != nil {
		r.Log.Error(err, fmt.Sprintf("failed to CreateOrUpdate the existing manifestwork %s", getManifestWorkKey(hyd)))
		return ctrl.Result{}, err

	}

	r.Log.Info(fmt.Sprintf("CreateOrUpdate manifestwork %s for hypershiftDeployment: %s at hostingCluster: %s", getManifestWorkKey(hyd), req, helper.GetHostingCluster(hyd)))

	setStatusCondition(
		hyd,
		hypdeployment.WorkConfigured,
		metav1.ConditionTrue,
		"",
		hypdeployment.ConfiguredAsExpectedReason,
	)

	return ctrl.Result{}, r.Client.Status().Patch(r.ctx, hyd, client.MergeFrom(inHyd))
}

func (r *HypershiftDeploymentReconciler) deleteManifestworkWaitCleanUp(ctx context.Context, hyd *hypdeployment.HypershiftDeployment) (ctrl.Result, error) {
	m, err := scaffoldManifestwork(hyd)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.Get(ctx, types.NamespacedName{Name: m.GetName(), Namespace: m.GetNamespace()}, m); err != nil {
		if apierrors.IsNotFound(err) {
			setStatusCondition(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse, "", hypdeployment.RemovingReason)
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to delete manifestwork, err: %v", err)
	}

	if m.GetDeletionTimestamp().IsZero() {
		patch := client.MergeFrom(m.DeepCopy())
		setManifestWorkSelectivelyDeleteOption(m, hyd)
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
	//caller will execute the status update
	setStatusCondition(hyd, hypdeployment.WorkConfigured, metav1.ConditionTrue, "Removing HypershiftDeployment's manifestwork and related resources", hypdeployment.RemovingReason)

	return ctrl.Result{RequeueAfter: 20 * time.Second, Requeue: true}, nil
}

func (r *HypershiftDeploymentReconciler) appendHostedClusterReferenceSecrets(ctx context.Context, providerSecret *corev1.Secret) loadManifest {
	log := r.Log

	return func(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
		var err error
		refSecrets := []*corev1.Secret{}

		// Get hostedcluster from manifestwork instead of hypD
		hostedCluster := getHostedClusterInManifestPayload(payload)
		if hostedCluster == nil {
			return err
		}

		hcSpec := &hostedCluster.Spec
		if len(hcSpec.PullSecret.Name) != 0 {
			var pullCreds *corev1.Secret
			if !hyd.Spec.Infrastructure.Configure {
				pullCreds, err = r.generateSecret(ctx,
					types.NamespacedName{Name: hcSpec.PullSecret.Name,
						Namespace: hyd.GetNamespace()})

				if err != nil {
					log.Error(err, "failed to duplicate pull secret")
					return err
				}
			} else {
				pullCreds = r.scaffoldPullSecret(hyd, *providerSecret)
			}

			refSecrets = append(refSecrets, pullCreds)
		}

		if hcSpec.Platform.AWS != nil {
			refSecrets = append(refSecrets, ScaffoldAWSSecrets(hyd, hostedCluster)...)
		} else if hcSpec.Platform.Azure != nil {
			creds, err := getAzureCloudProviderCreds(providerSecret)
			if err != nil {
				return nil
			}
			refSecrets = append(refSecrets, ScaffoldAzureCloudCredential(hyd, creds))
		}

		sshKey := hcSpec.SSHKey
		if len(sshKey.Name) != 0 {
			s, err := r.generateSecret(ctx,
				types.NamespacedName{Name: sshKey.Name,
					Namespace: hyd.GetNamespace()})

			if err != nil {
				log.Error(err, "failed to duplicate ssh secret")
				return err
			}

			refSecrets = append(refSecrets, s)
		}

		for _, s := range refSecrets {
			o := duplicateSecretWithOverride(s, overrideNamespace(helper.GetHostingNamespace(hyd)))
			*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: o}})
		}

		return nil
	}
}

func (r *HypershiftDeploymentReconciler) appendHostedCluster(ctx context.Context) loadManifest {
	return func(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {

		hc, err := r.scaffoldHostedCluster(ctx, hyd)
		if err != nil {
			return err
		}

		*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: hc}})

		return nil
	}
}

func ensureTaregetNamespace(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
	hostingNamespace := helper.GetHostingNamespace(hyd)
	*payload = append(*payload, workv1.Manifest{
		RawExtension: runtime.RawExtension{Object: &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: hostingNamespace,
			},
			TypeMeta: metav1.TypeMeta{
				Kind:       "Namespace",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
		}},
	})

	return nil
}

func (r *HypershiftDeploymentReconciler) appendNodePool(ctx context.Context) loadManifest {
	return func(hyd *hypdeployment.HypershiftDeployment, payload *[]workv1.Manifest) error {
		if !hyd.Spec.Infrastructure.Configure && len(hyd.Spec.NodePoolsRef) != 0 {
			npRefs := hyd.Spec.NodePoolsRef

			for _, npRef := range npRefs {
				gvr := schema.GroupVersionResource{
					Group:    "hypershift.openshift.io",
					Version:  "v1alpha1",
					Resource: "nodepools",
				}
				unstructNodePool, err := r.DynamicClient.Resource(gvr).Namespace(hyd.Namespace).Get(ctx, npRef.Name, metav1.GetOptions{})
				if err != nil {
					_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
						fmt.Sprintf("NodePoolRef %v:%v is not found", hyd.Namespace, npRef.Name), hypdeployment.MisConfiguredReason)

					return fmt.Errorf(fmt.Sprintf("failed to get NodePoolRef: %v:%v", hyd.Namespace, npRef.Name))
				}

				npObj := &hyp.NodePool{}
				err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructNodePool.UnstructuredContent(), npObj)
				if err != nil {
					_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.WorkConfigured, metav1.ConditionFalse,
						fmt.Sprintf("NodePoolRef %v:%v is invalid", hyd.Namespace, npRef.Name), hypdeployment.MisConfiguredReason)

					return fmt.Errorf(fmt.Sprintf("failed to validate Node Pool against current specs: %v:%v", hyd.Namespace, npRef.Name))
				}

				// Just use the spec from the nodepool object ref
				np := ScaffoldNodePool(hyd, npObj.Name, unstructNodePool.Object["spec"].(map[string]interface{}))
				*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: np}})
			}
		} else {
			for _, hdNp := range hyd.Spec.NodePools {
				usNpSpec, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&hdNp.Spec)
				if err != nil {
					return fmt.Errorf(fmt.Sprintf("failed to transform HypershiftDeployment.Spec.NodePools from hypershiftDeployment: %v:%v", hyd.Namespace, hdNp.Name))
				}

				np := ScaffoldNodePool(hyd, hdNp.Name, usNpSpec)
				*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: np}})
			}
		}

		return nil
	}
}

func getManifestWorkConfigs(hyd *hypdeployment.HypershiftDeployment) map[workv1.ResourceIdentifier]workv1.ManifestConfigOption {
	out := map[workv1.ResourceIdentifier]workv1.ManifestConfigOption{}
	k := workv1.ResourceIdentifier{
		Group:     hyp.GroupVersion.Group,
		Resource:  HostedClusterResource,
		Name:      hyd.Name,
		Namespace: helper.GetHostingNamespace(hyd),
	}

	out[k] = workv1.ManifestConfigOption{
		ResourceIdentifier: k,
		FeedbackRules: []workv1.FeedbackRule{
			{
				Type: workv1.JSONPathsType,
				// mirroring https://github.com/openshift/hypershift/blob/b9418cb392b94bc6682c76ce21b5dfd2744b9e8c/api/v1alpha1/hostedcluster_types.go#L1209
				JsonPaths: []workv1.JsonPath{
					{
						Name: Reason,
						Path: fmt.Sprintf(".status.conditions[?(@.type==\"Available\")].reason"),
					},
					{
						Name: StatusFlag,
						Path: fmt.Sprintf(".status.conditions[?(@.type==\"Available\")].status"),
					},
					{
						Name: Message,
						Path: fmt.Sprintf(".status.conditions[?(@.type==\"Available\")].message"),
					},
					{
						Name: Progress,
						Path: fmt.Sprintf(".status.version.history[?(@.state!=\"\")].state"),
					},
				},
			},
		},
	}

	for _, np := range hyd.Spec.NodePools {
		k := workv1.ResourceIdentifier{
			Group:     hyp.GroupVersion.Group,
			Resource:  NodePoolResource,
			Name:      np.Name,
			Namespace: helper.GetHostingNamespace(hyd),
		}

		out[k] = workv1.ManifestConfigOption{
			ResourceIdentifier: k,
			FeedbackRules: []workv1.FeedbackRule{
				{
					Type: workv1.JSONPathsType,
					JsonPaths: []workv1.JsonPath{
						{
							Name: Reason,
							Path: fmt.Sprintf(".status.conditions[?(@.type==\"Ready\")].reason"),
						},
						{
							Name: StatusFlag,
							Path: fmt.Sprintf(".status.conditions[?(@.type==\"Ready\")].status"),
						},
						{
							Name: Message,
							Path: fmt.Sprintf(".status.conditions[?(@.type==\"Ready\")].message"),
						},
					},
				},
			},
		}

	}

	return out
}

func feedbackToCondition(t hypdeployment.ConditionType, fvs []workv1.FeedbackValue) (metav1.Condition, bool) {
	m := metav1.Condition{
		Type: string(t),
	}

	switch t {
	case hypdeployment.HostedClusterProgress:
		for _, v := range fvs {
			if v.Name == Progress {
				m.Reason = string(*v.Value.String)
			}
		}

		m.Status = "True"
	default:
		for _, v := range fvs {
			if v.Name == Reason {
				m.Reason = string(*v.Value.String)
			}

			if v.Name == StatusFlag {
				m.Status = metav1.ConditionStatus(*v.Value.String)
			}

			if v.Name == Message {
				m.Message = string(*v.Value.String)
			}
		}
	}

	// field is missing or not update properly
	if len(m.Reason) == 0 || len(m.Status) == 0 {
		return metav1.Condition{}, false
	}

	return m, true
}

func enableManifestStatusFeedback(m *workv1.ManifestWork, hyd *hypdeployment.HypershiftDeployment) []workv1.ManifestConfigOption {
	if m == nil {
		return []workv1.ManifestConfigOption{}
	}

	cfg := []workv1.ManifestConfigOption{}

	cfgMap := getManifestWorkConfigs(hyd)

	for _, v := range cfgMap {
		cfg = append(cfg, v)
	}

	m.Spec.ManifestConfigs = cfg

	return cfg
}

func getStatusFeedbackAsCondition(m *workv1.ManifestWork, hyd *hypdeployment.HypershiftDeployment) []metav1.Condition {
	idMap := getManifestWorkConfigs(hyd)
	if m == nil || hyd == nil {
		return []metav1.Condition{}
	}

	out := []metav1.Condition{}

	for _, obj := range m.Status.ResourceStatus.Manifests {
		rMeta := resourceMeta(obj.ResourceMeta)
		id := rMeta.ToIdentifier()

		if _, ok := idMap[id]; !ok {
			continue
		}

		if id.Resource == NodePoolResource {
			npCond, ok := feedbackToCondition(hypdeployment.Nodepool, obj.StatusFeedbacks.Values)
			if !ok {
				continue
			}

			// find and set failed nodepool to condition
			st := condmeta.FindStatusCondition(out, string(hypdeployment.Nodepool))
			if st == nil {
				condmeta.SetStatusCondition(&out, npCond)
			} else if npCond.Status != "True" {
				condmeta.SetStatusCondition(&out, npCond)
			}
		}

		if id.Resource == HostedClusterResource {
			hcAvaCond, ok := feedbackToCondition(hypdeployment.HostedClusterAvailable, obj.StatusFeedbacks.Values)
			if ok {
				out = append(out, hcAvaCond)
			}

			hcProCond, ok := feedbackToCondition(hypdeployment.HostedClusterProgress, obj.StatusFeedbacks.Values)
			if ok {
				out = append(out, hcProCond)
			}
		}
	}

	// if there's nodepool condition and it's not false, then all nodepool are ready
	st := condmeta.FindStatusCondition(out, string(hypdeployment.Nodepool))
	if st != nil && st.Status == "True" {
		condmeta.SetStatusCondition(&out, metav1.Condition{
			Type:   string(hypdeployment.Nodepool),
			Status: "True",
			Reason: hypdeployment.NodePoolProvision,
		})
	}

	return out
}

func isFeedbackStatusFalse(fbs workv1.StatusFeedbackResult) bool {
	for _, v := range fbs.Values {
		//if there's not ready nodepool, report this one
		if v.Name == StatusFlag && *v.Value.String != "True" {
			return true
		}
	}

	return false
}

type resourceMeta workv1.ManifestResourceMeta

func (r resourceMeta) ToIdentifier() workv1.ResourceIdentifier {
	return workv1.ResourceIdentifier{
		Group:     r.Group,
		Resource:  r.Resource,
		Name:      r.Name,
		Namespace: r.Namespace,
	}
}

func getManifestPayloadSecretByName(manifests *[]workv1.Manifest, secretName string) (*corev1.Secret, error) {
	for _, v := range *manifests {
		if len(v.Raw) != 0 {
			u := &unstructured.Unstructured{}
			if err := json.Unmarshal(v.Raw, u); err != nil {
				return nil, err
			}

			if u.GetKind() == "Secret" && u.GetName() == secretName {
				secret := &corev1.Secret{}
				if err := json.Unmarshal(v.Raw, secret); err != nil {
					return nil, err
				}
				return secret, nil
			}
		} else if v.Object != nil && v.Object.GetObjectKind().GroupVersionKind().Kind == "Secret" &&
			v.Object.(*corev1.Secret).Name == secretName {
			return v.Object.(*corev1.Secret), nil
		}
	}

	return nil, nil
}

func getHostedClusterInManifestPayload(manifests *[]workv1.Manifest) *hyp.HostedCluster {
	for _, v := range *manifests {
		if v.Object.GetObjectKind().GroupVersionKind().Kind == "HostedCluster" {
			hostedCluster := &hyp.HostedCluster{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(v.Object.(*unstructured.Unstructured).UnstructuredContent(), hostedCluster)
			if err != nil {
				mLog.Error(err, "failed to convert unstructured HostedCluster to concrete type")
				return nil
			}

			return hostedCluster
		}
	}

	return nil
}

func getNodePoolsInManifestPayload(manifests *[]workv1.Manifest) []*hyp.NodePool {
	nodePools := []*hyp.NodePool{}
	for _, v := range *manifests {
		if v.Object.GetObjectKind().GroupVersionKind().Kind == "NodePool" {
			np := &hyp.NodePool{}
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(v.Object.(*unstructured.Unstructured).UnstructuredContent(), np)
			if err != nil {
				mLog.Error(err, "failed to convert unstructured NodePool to concrete type")
				return nil
			}

			nodePools = append(nodePools, np)
		}
	}

	return nodePools
}
