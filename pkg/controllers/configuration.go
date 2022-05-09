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

	apifixtures "github.com/openshift/hypershift/api/fixtures"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/pkg/errors"

	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	"github.com/stolostron/hypershift-deployment-controller/pkg/helper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	workv1 "open-cluster-management.io/api/work/v1"
)

type override func(obj metav1.Object)

func overrideNamespace(hostingNamespace string) override {
	return func(o metav1.Object) {
		if len(hostingNamespace) == 0 {
			return
		}
		o.SetNamespace(hostingNamespace)
	}
}

func duplicateSecretWithOverride(in *corev1.Secret, ops ...override) *corev1.Secret {
	out := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
	}

	out.SetName(in.GetName())
	out.SetLabels(in.GetLabels())
	out.Data = in.Data

	for _, o := range ops {
		o(out)
	}

	return out
}

func (r *HypershiftDeploymentReconciler) generateSecret(ctx context.Context, key types.NamespacedName, ops ...override) (*corev1.Secret, error) {
	origin := &corev1.Secret{}
	if err := r.Get(ctx, key, origin); err != nil {
		return nil, fmt.Errorf("failed to get the pull secret %v, err: %w", key, err)
	}

	return duplicateSecretWithOverride(origin, ops...), nil
}

func duplicateConfigMapWithOverride(in *corev1.ConfigMap, ops ...override) *corev1.ConfigMap {
	out := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
	}

	out.SetName(in.GetName())
	out.SetLabels(in.GetLabels())

	for _, o := range ops {
		o(out)
	}

	return out
}

func (r *HypershiftDeploymentReconciler) generateConfigMap(ctx context.Context, key types.NamespacedName, ops ...override) (*corev1.ConfigMap, error) {
	origin := &corev1.ConfigMap{}
	if err := r.Get(ctx, key, origin); err != nil {
		return nil, fmt.Errorf("failed to get the configMap, err: %w", err)
	}

	return duplicateConfigMapWithOverride(origin, ops...), nil
}

// make sure all configuration resources listed at https://github.com/stolostron/backlog/issues/20243
// are loaded to manifestwork
func (r *HypershiftDeploymentReconciler) ensureConfiguration(ctx context.Context, manifestwork *workv1.ManifestWork) loadManifest {
	var allErr []error
	return func(hyd *hypdeployment.HypershiftDeployment,
		payload *[]workv1.Manifest) error {

		type secretResource struct {
			secretRef        corev1.LocalObjectReference
			createSecretFunc func() (*corev1.Secret, error)
		}

		//source:
		// hyd.Spec.HostedClusterSpec.Configuration.SecretRefs
		// hyd.Spec.HostedClusterSpec.SecretEncryption.KMS.AWS.Auth
		// hyd.Spec.HostedClusterSpec.SecretEncryption.AESCBC.ActiveKey
		// hyd.Spec.HostedClusterSpec.SecretEncryption.AESCBC.BackupKey
		secretRefs := []secretResource{}

		//source:
		// hyd.Spec.HostedClusterSpec.Configuration.ConfigMapRefs
		// hyd.Spec.HostedClsuterSpec.AdditionalTrustBundle
		// hyd.Spec.NodePoolSpec.Config
		configMapRefs := []corev1.LocalObjectReference{}

		// Get hostedcluster from manifestwork instead of hypD
		hostedCluster := getHostedClusterInManifestPayload(payload)
		var hcSpec *hyp.HostedClusterSpec
		if hostedCluster != nil {
			hcSpec = &hostedCluster.Spec
		}

		if hcSpec != nil {
			hcSpecCfg := hcSpec.Configuration
			if hcSpecCfg != nil {
				if len(hcSpecCfg.SecretRefs) != 0 {
					for _, sref := range hcSpecCfg.SecretRefs {
						secretRefs = append(secretRefs, secretResource{secretRef: sref})
					}
				}

				if len(hcSpecCfg.ConfigMapRefs) != 0 {
					configMapRefs = append(configMapRefs, hcSpecCfg.ConfigMapRefs...)
				}
			}

			if hcSpec.SecretEncryption != nil {
				encr := hcSpec.SecretEncryption

				if encr.Type == hyp.AESCBC && encr.AESCBC != nil {
					if len(encr.AESCBC.ActiveKey.Name) != 0 {
						aesAKRef := secretResource{
							secretRef: encr.AESCBC.ActiveKey,
							createSecretFunc: func() (*corev1.Secret, error) {
								// Generate and scaffold the encryption secret
								exampleOptions := &apifixtures.ExampleOptions{
									Name:      hyd.Name,
									Namespace: helper.GetHostingNamespace(hyd),
								}
								encryptionSecret := exampleOptions.EtcdEncryptionKeySecret()
								if encryptionSecret != nil {
									encryptionSecret.Name = encr.AESCBC.ActiveKey.Name
									r.Log.Info(fmt.Sprintf("Generate etcd encryption secret: %v/%v", encryptionSecret.GetNamespace(), encryptionSecret.GetName()))
								}

								return encryptionSecret, nil
							},
						}

						secretRefs = append(secretRefs, aesAKRef)
					}

					if encr.AESCBC.BackupKey != nil && len(encr.AESCBC.BackupKey.Name) != 0 {
						secretRefs = append(secretRefs, secretResource{secretRef: *(encr.AESCBC.BackupKey)})
					}
				}

				// Pull in external KMS encryption secret
				if encr.Type == hyp.KMS && encr.KMS != nil && encr.KMS.AWS != nil && len(encr.KMS.AWS.Auth.Credentials.Name) != 0 {
					secretRefs = append(secretRefs, secretResource{secretRef: encr.KMS.AWS.Auth.Credentials})
				}
			}

			if hcSpec.AdditionalTrustBundle != nil && len(hcSpec.AdditionalTrustBundle.Name) != 0 {
				configMapRefs = append(configMapRefs, *hcSpec.AdditionalTrustBundle)
			}

			if hcSpec.ServiceAccountSigningKey != nil && len(hcSpec.ServiceAccountSigningKey.Name) != 0 {
				secretRefs = append(secretRefs, secretResource{secretRef: *hcSpec.ServiceAccountSigningKey})
			}

			// Get AWS secrets externally for configure=F and using objectRef
			if !hyd.Spec.Infrastructure.Configure && len(hyd.Spec.HostedClusterRef.Name) != 0 && hcSpec.Platform.AWS != nil {
				if len(hcSpec.Platform.AWS.ControlPlaneOperatorCreds.Name) != 0 {
					secretRefs = append(secretRefs, secretResource{secretRef: hcSpec.Platform.AWS.ControlPlaneOperatorCreds})
				}
				if len(hcSpec.Platform.AWS.KubeCloudControllerCreds.Name) != 0 {
					secretRefs = append(secretRefs, secretResource{secretRef: hcSpec.Platform.AWS.KubeCloudControllerCreds})
				}
				if len(hcSpec.Platform.AWS.NodePoolManagementCreds.Name) != 0 {
					secretRefs = append(secretRefs, secretResource{secretRef: hcSpec.Platform.AWS.NodePoolManagementCreds})
				}
			}
		}

		// Get nodepool from manifestwork instead of hypD
		nodepools := getNodePoolsInManifestPayload(payload)
		for _, np := range nodepools {
			if len(np.Spec.Config) != 0 {
				configMapRefs = append(configMapRefs, np.Spec.Config...)
			}
		}

		for _, se := range secretRefs {
			// 1. Use user provided secret
			k := genKey(se.secretRef, hyd)
			secret, err := r.generateSecret(ctx, k, overrideNamespace(helper.GetHostingNamespace(hyd)))
			if err != nil {
				r.Log.Info(fmt.Sprintf("did not find and copy secret %s: %s", k, err.Error()))
			}

			if secret == nil {
				// 2. Use existing secret in manifestwork payload
				secret, err = getManifestPayloadSecretByName(&manifestwork.Spec.Workload.Manifests, se.secretRef.Name)
				if err != nil {
					r.Log.Info(fmt.Sprintf("did not get secret %s from manifestwork: %s", se.secretRef.Name, err.Error()))
				}

				if secret == nil &&
					hyd.Spec.Infrastructure.Configure &&
					se.createSecretFunc != nil {
					// 3. For configure=T - Generate secret
					secret, err = se.createSecretFunc()
					if err != nil {
						r.Log.Error(err, fmt.Sprintf("failed to create secret %s", se.secretRef.Name))
					}
				}
			}

			if secret == nil {
				// 4. Fail if secret is not found/created
				err = errors.Errorf("failed to find/create secret %s", se.secretRef.Name)
				allErr = append(allErr, err)
				continue
			}
			*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: secret}})
		}

		for _, cm := range configMapRefs {
			k := genKey(cm, hyd)
			t, err := r.generateConfigMap(ctx, k, overrideNamespace(helper.GetHostingNamespace(hyd)))
			if err != nil {
				r.Log.Error(err, fmt.Sprintf("failed to copy secret %s", k))
				allErr = append(allErr, err)
				continue
			}

			*payload = append(*payload, workv1.Manifest{RawExtension: runtime.RawExtension{Object: t}})
		}

		return utilerrors.NewAggregate(allErr)
	}
}

func genKey(r corev1.LocalObjectReference, hyd *hypdeployment.HypershiftDeployment) types.NamespacedName {
	return types.NamespacedName{Name: r.Name, Namespace: hyd.GetNamespace()}
}
