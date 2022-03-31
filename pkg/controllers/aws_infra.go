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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openshift/hypershift/cmd/infra/aws"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
)

const (
	oidcStorageProvider        = "oidc-storage-provider-s3-config"
	oidcSPNamespace            = "kube-public"
	hypershiftBucketSecretName = "hypershift-operator-oidc-provider-s3-credentials"
)

func (r *HypershiftDeploymentReconciler) createAWSInfra(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (ctrl.Result, error) {

	var iamOut *aws.CreateIAMOutput
	oHyd := *hyd.DeepCopy()

	ctx := r.ctx
	log := r.Log

	if hyd.Spec.Infrastructure.Platform.AWS.Region == "" {
		return ctrl.Result{}, r.updateMissingInfrastructureParameterCondition(hyd, "Missing value HypershiftDeployment.Spec.Infrastructure.Platform.AWS.Region")
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

		infraOut, err := o.CreateInfra(r.ctx)
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
		ScaffoldAWSHostedClusterSpec(hyd, infraOut)
		ScaffoldAWSNodePoolSpec(hyd, infraOut)

		if err := r.patchHypershiftDeploymentResource(hyd, &oHyd); err != nil {
			_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
			return ctrl.Result{}, err
		}

		if err := r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Infrastructure configured")

		if err := r.Get(ctx, types.NamespacedName{Namespace: hyd.Namespace, Name: hyd.Name}, hyd); err != nil {
			return ctrl.Result{}, nil
		}

		oHyd = *hyd.DeepCopy()

		oidcSPName, oidcSPRegion, iamErr := oidcDiscoveryURL(r, hyd)
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
			if iamErr != nil {
				_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, iamErr.Error(), hypdeployment.MisConfiguredReason)
				return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true}, iamErr
			}

			hyd.Spec.HostedClusterSpec.IssuerURL = iamOut.IssuerURL
			hyd.Spec.HostedClusterSpec.Platform.AWS.Roles = iamOut.Roles
			hyd.Spec.Credentials = &hypdeployment.CredentialARNs{
				AWS: &hypdeployment.AWSCredentials{
					ControlPlaneOperatorARN: iamOut.ControlPlaneOperatorRoleARN,
					KubeCloudControllerARN:  iamOut.KubeCloudControllerRoleARN,
					NodePoolManagementARN:   iamOut.NodePoolManagementRoleARN,
				}}
			if err := r.patchHypershiftDeploymentResource(hyd, &oHyd); err != nil {
				return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, err.Error(), hypdeployment.MisConfiguredReason)
			}
			_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
			log.Info("IAM configured")

			if hyd.Spec.Override != hypdeployment.InfraConfigureWithManifest {
				if iamErr = r.createPullSecret(hyd, *providerSecret); iamErr == nil {
					_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, iamErr.Error(), hypdeployment.MisConfiguredReason)
					return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true}, iamErr
				}
				if iamErr = createOIDCSecrets(r, hyd); iamErr == nil {
					_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, iamErr.Error(), hypdeployment.MisConfiguredReason)
					return ctrl.Result{RequeueAfter: 1 * time.Minute, Requeue: true}, iamErr
				}
				log.Info("Secrets configured")
			}
		}
	}

	if hyd.Spec.HostedClusterSpec.Platform.Type == "AWS" && (hyd.Spec.Credentials == nil || hyd.Spec.Credentials.AWS == nil) {
		return ctrl.Result{}, r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Missing Spec.Credentials.AWS", hypdeployment.MisConfiguredReason)
	}
	return ctrl.Result{}, nil
}

func (r *HypershiftDeploymentReconciler) destroyAWSInfrastructure(hyd *hypdeployment.HypershiftDeployment, providerSecret *corev1.Secret) (reconcile.Result, error) {
	log := r.Log
	ctx := r.ctx

	dOpts := aws.DestroyInfraOptions{
		AWSCredentialsFile: "",
		AWSKey:             string(providerSecret.Data["aws_access_key_id"]),
		AWSSecretKey:       string(providerSecret.Data["aws_secret_access_key"]),
		Region:             hyd.Spec.Infrastructure.Platform.AWS.Region,
		BaseDomain:         string(providerSecret.Data["baseDomain"]),
		InfraID:            hyd.Spec.InfraID,
		Name:               hyd.GetName(),
	}

	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Removing AWS infrastructure with infra-id: "+hyd.Spec.InfraID, hypdeployment.PlatfromDestroyReason)

	log.Info("Deleting Infrastructure on provider")
	if err := dOpts.DestroyInfra(ctx); err != nil {
		log.Error(err, "there was a problem destroying infrastructure on the provider, retrying in 30s")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Info("Deleting Infrastructure IAM on provider")
	iamOpt := aws.DestroyIAMOptions{
		Region:       hyd.Spec.Infrastructure.Platform.AWS.Region,
		AWSKey:       dOpts.AWSKey,
		AWSSecretKey: dOpts.AWSSecretKey,
		InfraID:      dOpts.InfraID,
	}
	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Removing AWS IAM with infra-id: "+hyd.Spec.InfraID, hypdeployment.RemovingReason)

	if err := iamOpt.DestroyIAM(ctx); err != nil {
		log.Error(err, "failed to delete IAM on provider")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}

	log.Info("Deleting OIDC secrets")
	if err := destroySecrets(r, hyd); err != nil {
		log.Error(err, "Encountered an issue while deleting OIDC secrets")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, err
	}
	return ctrl.Result{}, nil
}

func oidcDiscoveryURL(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment) (string, string, error) {
	if hyd.Spec.Override == hypdeployment.InfraConfigureWithManifest {

		if len(hyd.Spec.HostingCluster) == 0 {
			err := errors.New(helper.HostingClusterMissing)
			r.Log.Error(err, "Spec.HostingCluster needs a ManagedCluster name")
			return "", "", err
		}

		// If the override is manifestwork that means we are using the hypershift created by mce hypershift-addon,
		// so there must exist a hypershift bucket secret in the management cluster namespace.
		secret := &corev1.Secret{}
		if err := r.Client.Get(context.Background(), types.NamespacedName{
			Name: hypershiftBucketSecretName, Namespace: helper.GetHostingCluster(hyd)}, secret); err != nil {
			return "", "", err
		}

		return string(secret.Data["bucket"]), string(secret.Data["region"]), nil
	}

	cm := &corev1.ConfigMap{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{Name: oidcStorageProvider, Namespace: oidcSPNamespace}, cm); err != nil {
		return "", "", err
	}
	return cm.Data["name"], cm.Data["region"], nil
}
