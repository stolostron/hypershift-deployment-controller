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
	"encoding/json"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hypershift/api/fixtures"
	"github.com/openshift/hypershift/cmd/infra/azure"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
)

const CCredsSuffix = "-cloud-credentials" // #nosec G101

func (r *HypershiftDeploymentReconciler) createAzureInfra(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {
	oHyd := *hyd.DeepCopy()
	log := r.Log

	if hyd.Spec.Infrastructure.Platform.Azure.Location == "" {
		return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform.Azure.Region")
	}

	// Skip reconcile based on condition
	// Does both INFRA
	var o azure.CreateInfraOptions
	credentials, err := getAzureCloudProviderCreds(providerSecret)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured)) {

		log.Info("Creating infrastructure in Azure that will be used by the HypershiftDeployment, HostedClusters & NodePools")
		o = azure.CreateInfraOptions{

			Location:    hyd.Spec.Infrastructure.Platform.Azure.Location,
			InfraID:     hyd.Spec.InfraID,
			Name:        hyd.GetName(),
			BaseDomain:  string(providerSecret.Data["baseDomain"]),
			Credentials: credentials,
		}

		infraOut, err := o.Run(r.ctx)
		if err != nil {
			log.Error(err, "Could not create infrastructure")

			return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true},
				r.updateStatusConditionsOnChange(
					hyd, hypdeployment.PlatformConfigured,
					metav1.ConditionFalse,
					err.Error(),
					hypdeployment.MisConfiguredReason)
		}

		// This creates the required HostedClusterSpec and NodePoolSpec(s), from scratch or if supplied
		ScaffoldAzureHostedClusterSpec(hyd, infraOut)
		hyd.Spec.HostedClusterSpec.Platform.Azure.SubscriptionID = o.Credentials.SubscriptionID
		ScaffoldAzureNodePoolSpec(hyd, infraOut)

		if err := r.patchHypershiftDeploymentResource(hyd, &oHyd); err != nil {
			_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
			return ctrl.Result{}, err
		}

		if err := r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Infrastructure configured")
	}
	if !meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformIAMConfigured)) {
		if hyd.Spec.Override != hypdeployment.InfraConfigureWithManifest {
			r.Log.Info("Creating cloud credential secret")
			cloudCredSecret := ScaffoldAzureCloudCredential(hyd, credentials)
			if _, err := controllerutil.CreateOrUpdate(r.ctx, r.Client, cloudCredSecret, func() error { return nil }); err != nil {
				return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Could not create or update the cloud credential secret", hypdeployment.MisConfiguredReason)
			}
		}
		// Todo, this should be skipped and the manifestwork should generate it from a providerCredential
		log.Info("Creating pull secret")
		if err := r.createPullSecret(hyd, *providerSecret); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Complete IAM configuration")
		_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
	}
	return ctrl.Result{}, nil
}

func (r *HypershiftDeploymentReconciler) destroyAzureInfrastructure(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (reconcile.Result, error) {
	log := r.Log
	ctx := r.ctx

	dOpts := azure.DestroyInfraOptions{
		Location:    hyd.Spec.Infrastructure.Platform.Azure.Location,
		Credentials: &fixtures.AzureCreds{},
		Name:        hyd.Name,
		InfraID:     hyd.Spec.InfraID,
	}

	if err := json.Unmarshal(providerSecret.Data["osServicePrincipal.json"], &dOpts.Credentials); err != nil {
		return ctrl.Result{}, err
	}
	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Removing Azure infrastructure with infra-id: "+hyd.Spec.InfraID, hypdeployment.PlatfromDestroyReason)

	log.Info("Deleting Infrastructure on provider")
	if err := dOpts.Run(ctx); err != nil {
		log.Error(err, "there was a problem destroying infrastructure on the provider, retrying in 30s")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Info("Deleting Azure cloud credentials  secrets")
	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Removing Azure cloud credentials", hypdeployment.RemovingReason)
	if err := destroySecrets(r, hyd); err != nil {
		log.Error(err, "Encountered an issue while deleting OIDC secrets")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	return ctrl.Result{}, nil
}

func getAzureCloudProviderCreds(providerSecret *corev1.Secret) (*fixtures.AzureCreds, error) {
	credentials := &fixtures.AzureCreds{}
	err := json.Unmarshal(providerSecret.Data["osServicePrincipal.json"], &credentials)
	return credentials, err
}
