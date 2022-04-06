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

package v1alpha1

import (
	hypv1alpha1 "github.com/openshift/hypershift/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ConditionType string

type CurrentPhase string

type InfraOverride string

const (
	ConfiguredAsExpectedReason = "ConfiguredAsExpected"
	PlatfromDestroyReason      = "Destroying"
	MisConfiguredReason        = "MisConfigured"
	BeingConfiguredReason      = "BeingConfigured"
	RemovingReason             = "Removing"
	AsExpectedReason           = "AsExpected"
	NodePoolProvision          = "NodePoolsProvisioned"

	// PlatformConfigured indicates (if status is true) that the
	// platform configuration specified for the platform provider has been applied
	PlatformConfigured ConditionType = "PlatformInfrastructureConfigured"
	// PlatformIAMConfigured indicates (if status is true) that the IAM is configured
	PlatformIAMConfigured ConditionType = "PlatformIAMConfigured"
	// ProviderSecretConfigured indicates the state of the secret reference
	ProviderSecretConfigured ConditionType = "ProviderSecretConfigured"

	// HostedCluster indicates the state of the hostedcluster
	HostedClusterAvaliable ConditionType = "HostedClusterAvaliable"

	// HostedCluster indicates the state of the hostedcluster
	HostedClusterProgress ConditionType = "HostedClusterProgress"

	// Nodepool indicates the state of the nodepools
	Nodepool ConditionType = "NodePool"

	// this mirror open-cluster-management.io/api/work/v1/types.go#L266-L279
	// WorkProgressing represents that the work is in the progress to be
	// applied on the managed cluster.
	WorkProgressing ConditionType = "Progressing"
	// WorkApplied represents that the workload defined in work is
	// succesfully applied on the managed cluster.
	WorkApplied ConditionType = "Applied"
	// WorkAvailable represents that all resources of the work exists on
	// the managed cluster.
	WorkAvailable ConditionType = "Available"
	// WorkDegraded represents that the current state of work does not match
	// the desired state for a certain period.
	WorkDegraded ConditionType = "Degraded"
	// WorkConfigured indicates the status of applying the ManifestWork
	WorkConfigured ConditionType = "ManifestWorkConfigured"

	InfraOverrideDestroy   = "ORPHAN"
	InfraConfigureOnly     = "INFRA-ONLY"
	DeleteHostingNamespace = "DELETE-HOSTING-NAMESPACE"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// HypershiftDeploymentSpec defines the desired state of HypershiftDeployment
type HypershiftDeploymentSpec struct {
	// Infrastructure instructions and pointers so either ClusterDeployment generates what is needed or
	// skips it when the user provides the infrastructure values
	// +immutable
	Infrastructure InfraSpec `json:"infrastructure"`

	// Infrastructure ID, this is used to tag resources in the Cloud Provider, it will be generated
	// if not provided
	// +immutable
	// +optional
	InfraID string `json:"infra-id,omitempty"`

	// InfrastructureOverride allows support for special cases
	//   OverrideDestroy = "ORPHAN"
	//   InfraConfigureOnly = "INFRA-ONLY"
	//   DeleteHostingNamespace = "DELETE-HOSTING-NAMESPACE"
	// +kubebuilder:validation:Enum=ORPHAN;INFRA-ONLY;DELETE-HOSTING-NAMESPACE
	Override InfraOverride `json:"override,omitempty"`

	//HostingNamespace specify the where the children resouces(hostedcluster, nodepool)
	//to sit in
	//if not provided, the default is "clusters"
	// +optional
	HostingNamespace string `json:"hostingNamespace"`

	//HostingCluster only applies to ManifestWork, and specifies which managedCluster's namespace the manifestwork will be applied to.
	//If not specified, the controller will flag an error condition.
	//The HostingCluster would be the management cluster of the hostedcluster and nodepool generated
	//by the hypershiftDeployment
	// +optional
	HostingCluster string `json:"hostingCluster"`

	// HostedCluster that will be applied to the ManagementCluster by ACM, if omitted, it will be generated
	// +optional
	HostedClusterSpec *hypv1alpha1.HostedClusterSpec `json:"hostedClusterSpec,omitempty"`

	// NodePools is an array of NodePool resources that will be applied to the ManagementCluster by ACM,
	// if omitted, a default NodePool will be generated
	// +optional
	NodePools []*HypershiftNodePools `json:"nodePools,omitempty"`

	// Credentials are ARN's that are used for standing up the resources in the cluster.
	Credentials *CredentialARNs `json:"credentials,omitempty"`
}

type CredentialARNs struct {
	AWS *AWSCredentials `json:"aws,omitempty"`
}

type AWSCredentials struct {
	ControlPlaneOperatorARN string `json:"controlPlaneOperatorARN"`
	KubeCloudControllerARN  string `json:"kubeCloudControllerARN"`
	NodePoolManagementARN   string `json:"nodePoolManagementARN"`
}

type HypershiftNodePools struct {
	// Name is the name to give this NodePool
	Name string `json:"name"`

	// Spec stores the NodePoolSpec you wan to use. If omitted, it will be generated
	Spec hypv1alpha1.NodePoolSpec `json:"spec"`
}

type InfraSpec struct {
	// Configure the infrastructure using the provided CloudProvider, or user provided
	// +immutable
	Configure bool `json:"configure"`

	// Region is the AWS region in which the cluster resides. This configures the
	// OCP control plane cloud integrations, and is used by NodePool to resolve
	// the correct boot AMI for a given release.
	//
	// +optional
	// +immutable
	Platform *Platforms `json:"platform,omitempty"`

	// CloudProvider secret, contains the Cloud credenetial, Pull Secret and Base Domain
	CloudProvider corev1.LocalObjectReference `json:"cloudProvider"`
}

type Platforms struct {
	Azure *AzurePlatform `json:"azure,omitempty"`
	AWS   *AWSPlatform   `json:"aws,omitempty"`
}

type AzurePlatform struct {

	// Region is the Azure region(location) in which the cluster resides. This configures the
	// OCP control plane cloud integrations, and is used by NodePool to resolve
	// the correct boot image for a given release.
	//
	// +immutable
	Location string `json:"location"`
}

type AWSPlatform struct {

	// Region is the AWS region in which the cluster resides. This configures the
	// OCP control plane cloud integrations, and is used by NodePool to resolve
	// the correct boot AMI for a given release.
	//
	// +immutable
	Region string `json:"region"`
}

// HypershiftDeploymentStatus defines the observed state of HypershiftDeployment
type HypershiftDeploymentStatus struct {
	// Track the conditions for each step in the desired curation that is being
	// executed as a job
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	//Show which phase of curation is currently being processed
	Phase CurrentPhase `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=hypershiftdeployments,shortName=hd;hds,scope=Namespaced
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="TYPE",type="string",JSONPath=".spec.hostedClusterSpec.platform.type",description="Infrastructure type"
// +kubebuilder:printcolumn:name="INFRA",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformInfrastructureConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformInfrastructureConfigured\")].status",description="Configured"
// +kubebuilder:printcolumn:name="IAM",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformIAMConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformIAMConfigured\")].status",description="Configured"
// +kubebuilder:printcolumn:name="MANIFESTWORK",type="string",JSONPath=".status.conditions[?(@.type==\"ManifestWorkConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="PROVIDER REF",type="string",JSONPath=".status.conditions[?(@.type==\"ProviderSecretConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="Found",type="string",JSONPath=".status.conditions[?(@.type==\"ProviderSecretConfigured\")].status",description="Found"
// +kubebuilder:printcolumn:name="HOSTING",type="string",JSONPath=".status.conditions[?(@.type==\"HostedClusterProgress\")].reason",description="Reason"

// HypershiftDeployment is the Schema for the hypershiftDeployments API
type HypershiftDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HypershiftDeploymentSpec   `json:"spec,omitempty"`
	Status HypershiftDeploymentStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// HypershiftDeploymentList contains a list of HypershiftDeployment
type HypershiftDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HypershiftDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HypershiftDeployment{}, &HypershiftDeploymentList{})
}
