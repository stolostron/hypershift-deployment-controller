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
	"github.com/stolostron/hypershift-deployment-controller/pkg/constant"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
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
		infraOut, err := r.InfraHandler.AwsInfraCreator(
			string(providerSecret.Data["aws_access_key_id"]),
			string(providerSecret.Data["aws_secret_access_key"]),
			hyd.Spec.Infrastructure.Platform.AWS.Region,
			hyd.Spec.InfraID,
			hyd.GetName(),
			string(providerSecret.Data["baseDomain"]),
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

		// This creates the required HostedClusterSpec and NodePoolSpec(s), from scratch if not supplied
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
			iamOut, iamErr = r.InfraHandler.AwsIAMCreator(
				string(providerSecret.Data["aws_access_key_id"]),
				string(providerSecret.Data["aws_secret_access_key"]),
				hyd.Spec.Infrastructure.Platform.AWS.Region,
				hyd.Spec.InfraID,
				oidcSPName,
				oidcSPRegion,
				infraOut.PrivateZoneID,
				infraOut.PublicZoneID,
				infraOut.LocalZoneID,
			)(r.ctx, r.Client)
			if iamErr != nil {
				_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured,
					metav1.ConditionFalse,
					iamErr.Error(),
					hypdeployment.MisConfiguredReason)
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
				return ctrl.Result{},
					r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured,
						metav1.ConditionFalse,
						err.Error(),
						hypdeployment.MisConfiguredReason)
			}
			_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionTrue, "", hypdeployment.ConfiguredAsExpectedReason)
			log.Info("IAM configured")
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
	awsKey := string(providerSecret.Data["aws_access_key_id"])
	awsSecretKey := string(providerSecret.Data["aws_secret_access_key"])

	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformConfigured, metav1.ConditionFalse, "Removing AWS infrastructure with infra-id: "+hyd.Spec.InfraID, hypdeployment.PlatfromDestroyReason)

	log.Info("Deleting Infrastructure on provider")

	if err := r.InfraHandler.AwsInfraDestroyer(
		awsKey,
		awsSecretKey,
		hyd.Spec.Infrastructure.Platform.AWS.Region,
		hyd.Spec.InfraID,
		hyd.GetName(),
		string(providerSecret.Data["baseDomain"]),
	)(ctx); err != nil {
		log.Error(err, "there was a problem destroying infrastructure on the provider, retrying in 30s")
		return ctrl.Result{RequeueAfter: 30 * time.Second},
			r.updateStatusConditionsOnChange(
				hyd, hypdeployment.PlatformConfigured,
				metav1.ConditionFalse,
				err.Error(),
				hypdeployment.PlatfromDestroyReason)
	}

	log.Info("Deleting Infrastructure IAM on provider")
	_ = r.updateStatusConditionsOnChange(hyd, hypdeployment.PlatformIAMConfigured, metav1.ConditionFalse, "Removing AWS IAM with infra-id: "+hyd.Spec.InfraID, hypdeployment.RemovingReason)

	if err := r.InfraHandler.AwsIAMDestroyer(
		awsKey,
		awsSecretKey,
		hyd.Spec.Infrastructure.Platform.AWS.Region,
		hyd.Spec.InfraID,
	)(ctx); err != nil {
		log.Error(err, "failed to delete IAM on provider")
		return ctrl.Result{RequeueAfter: 30 * time.Second},
			r.updateStatusConditionsOnChange(
				hyd, hypdeployment.PlatformIAMConfigured,
				metav1.ConditionFalse,
				err.Error(),
				hypdeployment.RemovingReason)
	}

	return ctrl.Result{}, nil
}

func oidcDiscoveryURL(r *HypershiftDeploymentReconciler, hyd *hypdeployment.HypershiftDeployment) (string, string, error) {

	if len(hyd.Spec.HostingCluster) == 0 {
		err := errors.New(constant.HostingClusterMissing)
		r.Log.Error(err, "Spec.HostingCluster needs a ManagedCluster name")
		return "", "", err
	}

	// If the override is manifestwork that means we are using the hypershift created by mce hypershift-addon,
	// so there must exist a hypershift bucket secret in the management cluster namespace.
	secret := &corev1.Secret{}
	if err := r.Client.Get(context.Background(), types.NamespacedName{
		Name: constant.HypershiftBucketSecretName, Namespace: helper.GetHostingCluster(hyd)}, secret); err != nil {
		return "", "", err
	}

	return string(secret.Data["bucket"]), string(secret.Data["region"]), nil
}
