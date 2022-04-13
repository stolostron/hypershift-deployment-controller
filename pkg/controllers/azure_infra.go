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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hypershift/api/fixtures"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
)

const CCredsSuffix = "-cloud-credentials" // #nosec G101

func (r *HypershiftDeploymentReconciler) createAzureInfra(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {
	oHyd := *hyd.DeepCopy()
	log := r.Log

	if hyd.Spec.Infrastructure.Platform.Azure.Location == "" {
		return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform.Azure.Location")
	}

	// Skip reconcile based on condition
	// Does both INFRA
	credentials, err := getAzureCloudProviderCreds(providerSecret)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !meta.IsStatusConditionTrue(hyd.Status.Conditions, string(hypdeployment.PlatformConfigured)) {
		log.Info("Creating infrastructure in Azure that will be used by the HypershiftDeployment, HostedClusters & NodePools")

		infraOut, err := r.InfraHandler.AzureInfraCreator(
			hyd.GetName(),
			string(providerSecret.Data["baseDomain"]),
			hyd.Spec.Infrastructure.Platform.Azure.Location,
			hyd.Spec.InfraID,
			credentials,
		)(r.ctx)
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
		hyd.Spec.HostedClusterSpec.Platform.Azure.SubscriptionID = credentials.SubscriptionID
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
		// If it can run to here, the credential is legal. we will copy the credential by manifestwork to the hosting cluster.
		log.Info("IAM configured")
		_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
	}
	return ctrl.Result{}, nil
}

func (r *HypershiftDeploymentReconciler) destroyAzureInfrastructure(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (reconcile.Result, error) {
	log := r.Log
	ctx := r.ctx

	credentials := &fixtures.AzureCreds{}
	if err := json.Unmarshal(providerSecret.Data["osServicePrincipal.json"], credentials); err != nil {
		return ctrl.Result{}, err
	}
	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Removing Azure infrastructure with infra-id: "+hyd.Spec.InfraID, hypdeployment.PlatfromDestroyReason)

	log.Info("Deleting Infrastructure on provider")
	if err := r.InfraHandler.AzureInfraDestroyer(
		hyd.Name,
		hyd.Spec.Infrastructure.Platform.Azure.Location,
		hyd.Spec.InfraID,
		credentials,
	)(ctx); err != nil {
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
