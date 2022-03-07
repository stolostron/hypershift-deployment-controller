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
	"fmt"

	"github.com/openshift/hypershift/api/fixtures"
	hyp "github.com/openshift/hypershift/api/v1alpha1"
	"github.com/openshift/hypershift/cmd/infra/aws"
	"github.com/openshift/hypershift/cmd/infra/azure"
	"github.com/openshift/hypershift/cmd/util"
	hypdeployment "github.com/stolostron/hypershift-deployment-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
)

var resLog = ctrl.Log.WithName("resource-render")

const ReleaseImage = "quay.io/openshift-release-dev/ocp-release:4.9.15-x86_64"

func getTargetNamespace(hyd *hypdeployment.HypershiftDeployment) string {
	t := hyd.GetNamespace()
	defer func() { resLog.Info(fmt.Sprintf("targetNamespace is: %s", t)) }()

	if len(hyd.Spec.TargetNamespace) == 0 {
		return t
	}

	t = hyd.Spec.TargetNamespace

	return t
}

func ScaffoldHostedCluster(hyd *hypdeployment.HypershiftDeployment) *hyp.HostedCluster {
	return &hyp.HostedCluster{
		ObjectMeta: v1.ObjectMeta{
			Name:      hyd.Name,
			Namespace: getTargetNamespace(hyd),
		},
		Spec: *hyd.Spec.HostedClusterSpec,
	}
}

// Creates an instance of ServicePublishingStrategyMapping
func spsMap(service hyp.ServiceType, psType hyp.PublishingStrategyType) hyp.ServicePublishingStrategyMapping {
	return hyp.ServicePublishingStrategyMapping{
		Service: service,
		ServicePublishingStrategy: hyp.ServicePublishingStrategy{
			Type: hyp.PublishingStrategyType(psType),
		},
	}
}

func ScaffoldAzureHostedClusterSpec(hyd *hypdeployment.HypershiftDeployment, infraOut *azure.CreateInfraOutput) {
	scaffoldHostedClusterSpec(hyd)
	ap := &hyp.AzurePlatformSpec{}
	ap.Location = infraOut.Location
	ap.MachineIdentityID = infraOut.MachineIdentityID
	ap.ResourceGroupName = infraOut.ResourceGroupName
	ap.SecurityGroupName = infraOut.SecurityGroupName
	ap.SubnetName = infraOut.SubnetName
	ap.VnetID = infraOut.VNetID
	ap.VnetName = infraOut.VnetName
	ap.Credentials.Name = hyd.Name + CCredsSuffix //This is generated and the secret is created below
	hyd.Spec.HostedClusterSpec.DNS = *scaffoldDnsSpec(infraOut.BaseDomain, infraOut.PrivateZoneID, infraOut.PublicZoneID)
	hyd.Spec.HostedClusterSpec.Platform.Azure = ap
	hyd.Spec.HostedClusterSpec.Platform.Type = hyp.AzurePlatform
}

func ScaffoldAWSHostedClusterSpec(hyd *hypdeployment.HypershiftDeployment, infraOut *aws.CreateInfraOutput) {
	scaffoldHostedClusterSpec(hyd)
	hyd.Spec.HostedClusterSpec.DNS = *scaffoldDnsSpec(infraOut.BaseDomain, infraOut.PrivateZoneID, infraOut.PublicZoneID)
	hyd.Spec.HostedClusterSpec.InfraID = hyd.Spec.InfraID
	hyd.Spec.HostedClusterSpec.Networking.MachineCIDR = infraOut.ComputeCIDR
	ap := &hyp.AWSPlatformSpec{
		Region:                    hyd.Spec.Infrastructure.Platform.AWS.Region,
		ControlPlaneOperatorCreds: corev1.LocalObjectReference{Name: hyd.Name + "-cpo-creds"},
		KubeCloudControllerCreds:  corev1.LocalObjectReference{Name: hyd.Name + "-cloud-ctrl-creds"},
		NodePoolManagementCreds:   corev1.LocalObjectReference{Name: hyd.Name + "-node-mgmt-creds"},
		EndpointAccess:            hyp.Public,
	}
	hyd.Spec.HostedClusterSpec.Platform.AWS = ap
	hyd.Spec.HostedClusterSpec.Platform.AWS.CloudProviderConfig = scaffoldCloudProviderConfig(infraOut)
	hyd.Spec.HostedClusterSpec.Platform.Type = hyp.AWSPlatform
}

func scaffoldHostedClusterSpec(hyd *hypdeployment.HypershiftDeployment) {
	volSize := resource.MustParse("4Gi")
	//releaseImage, _ := version.LookupDefaultOCPVersion()

	if hyd.Spec.HostedClusterSpec == nil {
		hyd.Spec.HostedClusterSpec =
			&hyp.HostedClusterSpec{
				InfraID:                      hyd.Spec.InfraID,
				ControllerAvailabilityPolicy: hyp.SingleReplica,
				Etcd: hyp.EtcdSpec{
					Managed: &hyp.ManagedEtcdSpec{
						Storage: hyp.ManagedEtcdStorageSpec{
							PersistentVolume: &hyp.PersistentVolumeEtcdStorageSpec{
								Size: &volSize,
							},
							Type: hyp.PersistentVolumeEtcdStorage,
						},
					},
					ManagementType: hyp.Managed,
				},
				FIPS: false,

				//IssuerURL: iamOut.IssuerURL,
				Networking: hyp.ClusterNetworking{
					ServiceCIDR: "172.31.0.0/16",
					PodCIDR:     "10.132.0.0/14",
					MachineCIDR: "", //This is overwritten below
					NetworkType: hyp.OpenShiftSDN,
				},
				// Defaults for all platforms
				PullSecret: corev1.LocalObjectReference{Name: hyd.Name + "-pull-secret"},
				Release: hyp.Release{
					Image: ReleaseImage, //.DownloadURL,
				},
				Services: []hyp.ServicePublishingStrategyMapping{
					spsMap(hyp.APIServer, hyp.LoadBalancer),
					spsMap(hyp.OAuthServer, hyp.Route),
					spsMap(hyp.OIDC, hyp.S3),
					spsMap(hyp.Konnectivity, hyp.Route),
					spsMap(hyp.Ignition, hyp.Route),
				},
			}
	}
}

func scaffoldDnsSpec(baseDomain string, privateZoneID string, publicZoneID string) *hyp.DNSSpec {
	return &hyp.DNSSpec{
		BaseDomain:    baseDomain,
		PrivateZoneID: privateZoneID,
		PublicZoneID:  publicZoneID,
	}
}

func scaffoldCloudProviderConfig(infraOut *aws.CreateInfraOutput) *hyp.AWSCloudProviderConfig {
	return &hyp.AWSCloudProviderConfig{
		Subnet: &hyp.AWSResourceReference{
			ID: &infraOut.Zones[0].SubnetID,
		},
		VPC:  infraOut.VPCID,
		Zone: infraOut.Zones[0].Name,
	}
}

func ScaffoldAzureNodePoolSpec(hyd *hypdeployment.HypershiftDeployment, infraOut *azure.CreateInfraOutput) {
	ScaffoldNodePoolSpec(hyd)
	for _, np := range hyd.Spec.NodePools {
		np.Spec.Platform.Type = hyp.AzurePlatform
		if np.Spec.Platform.Azure == nil {
			np.Spec.Platform.Azure = &hyp.AzureNodePoolPlatform{
				VMSize:  "Standard_D4s_v4",
				ImageID: infraOut.BootImageID,
			}
		}
	}
}

func ScaffoldAWSNodePoolSpec(hyd *hypdeployment.HypershiftDeployment, infraOut *aws.CreateInfraOutput) {
	ScaffoldNodePoolSpec(hyd)
	for _, np := range hyd.Spec.NodePools {
		np.Spec.Platform.Type = hyp.AWSPlatform
		if np.Spec.Platform.AWS == nil {
			np.Spec.Platform.AWS = scaffoldAWSNodePoolPlatform(infraOut)
		}
		if np.Spec.Platform.AWS.InstanceProfile == "" {
			np.Spec.Platform.AWS.InstanceProfile = hyd.Spec.InfraID + "-worker"
		}
		if np.Spec.Platform.AWS.Subnet == nil {
			np.Spec.Platform.AWS.Subnet = &hyp.AWSResourceReference{
				ID: &infraOut.Zones[0].SubnetID,
			}
		}
		if np.Spec.Platform.AWS.SecurityGroups == nil {
			np.Spec.Platform.AWS.SecurityGroups = []hyp.AWSResourceReference{
				hyp.AWSResourceReference{
					ID: &infraOut.SecurityGroupID,
				},
			}
		}
	}
}

func ScaffoldNodePoolSpec(hyd *hypdeployment.HypershiftDeployment) {

	nodeCount := int32(2)

	if len(hyd.Spec.NodePools) == 0 {
		hyd.Spec.NodePools = []*hypdeployment.HypershiftNodePools{
			&hypdeployment.HypershiftNodePools{
				Name: hyd.Name,
				Spec: hyp.NodePoolSpec{
					ClusterName: hyd.Name,
					Management: hyp.NodePoolManagement{
						AutoRepair: false,
						Replace: &hyp.ReplaceUpgrade{
							RollingUpdate: &hyp.RollingUpdate{
								MaxSurge:       &intstr.IntOrString{IntVal: 1},
								MaxUnavailable: &intstr.IntOrString{IntVal: 0},
							},
							Strategy: hyp.UpgradeStrategyRollingUpdate,
						},
						UpgradeType: hyp.UpgradeTypeReplace,
					},
					NodeCount: &nodeCount,
					Platform: hyp.NodePoolPlatform{
						Type: hyp.NonePlatform,
					},
					Release: hyp.Release{
						Image: ReleaseImage, //.DownloadURL,,
					},
				},
			},
		}
	}

	for _, np := range hyd.Spec.NodePools {
		if np.Spec.ClusterName != hyd.Name {
			np.Spec.ClusterName = hyd.Name
		}
	}
}

func scaffoldAWSNodePoolPlatform(infraOut *aws.CreateInfraOutput) *hyp.AWSNodePoolPlatform {
	volSize := int64(35)

	return &hyp.AWSNodePoolPlatform{
		InstanceType: "t3.large",
		RootVolume: &hyp.Volume{
			Size: volSize,
			Type: "gp3",
		},
	}
}

func ScaffoldNodePool(hyd *hypdeployment.HypershiftDeployment, np *hypdeployment.HypershiftNodePools) *hyp.NodePool {

	return &hyp.NodePool{
		ObjectMeta: v1.ObjectMeta{
			Name:      np.Name,
			Namespace: getTargetNamespace(hyd),
			Labels: map[string]string{
				AutoInfraLabelName: hyd.Spec.InfraID,
			},
		},
		Spec: np.Spec,
	}
}

func ScaffoldSecrets(hyd *hypdeployment.HypershiftDeployment) []*corev1.Secret {
	var secrets []*corev1.Secret

	buildAWSCreds := func(name, arn string) *corev1.Secret {
		return &corev1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "Secret",
				APIVersion: corev1.SchemeGroupVersion.String(),
			},
			ObjectMeta: metav1.ObjectMeta{
				Namespace: getTargetNamespace(hyd),
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
	return append(
		secrets,
		buildAWSCreds(hyd.Name+"-cpo-creds", hyd.Spec.Credentials.AWS.ControlPlaneOperatorARN),
		buildAWSCreds(hyd.Name+"-cloud-ctrl-creds", hyd.Spec.Credentials.AWS.KubeCloudControllerARN),
		buildAWSCreds(hyd.Name+"-node-mgmt-creds", hyd.Spec.Credentials.AWS.NodePoolManagementARN),
	)
}

func ScaffoldAzureCloudCredential(hyd *hypdeployment.HypershiftDeployment, creds *fixtures.AzureCreds) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hyd.Name + CCredsSuffix,
			Namespace: getTargetNamespace(hyd),
			Labels: map[string]string{
				util.AutoInfraLabelName: hyd.Spec.InfraID,
			},
		},
		Data: map[string][]byte{
			"AZURE_SUBSCRIPTION_ID": []byte(creds.SubscriptionID),
			"AZURE_TENANT_ID":       []byte(creds.TenantID),
			"AZURE_CLIENT_ID":       []byte(creds.ClientID),
			"AZURE_CLIENT_SECRET":   []byte(creds.ClientSecret),
		},
	}
}
