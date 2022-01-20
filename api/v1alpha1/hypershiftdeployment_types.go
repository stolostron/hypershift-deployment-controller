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

const (
	PlatformBeingConfigured         = "PlatformInfrastructureBeingConfigured"
	PlatformConfiguredAsExpected    = "PlatformInfrastructureConfiguredAsExpected"
	PlatfromDestroy                 = "PlatformInfrastructureDestroy"
	PlatformMisConfiguredReason     = "PlatformInfrastructureMisconfigured"
	PlatformIAMBeingConfigured      = "PlatformIAMBeingConfigured"
	PlatformIAMConfiguredAsExpected = "PlatformIAMConfiguredAsExpected"
	PlatformIAMRemove               = "PlatformIAMRemove"
	PlatformIAMMisConfiguredReason  = "PlatformIAMMisconfigured"

	// PlatformConfigured indicates (if status is true) that the
	// platform configuration specified for the platform provider has been deployed
	PlatformConfigured    ConditionType = "PlatformInfrastructureConfigured"
	PlatformIAMConfigured ConditionType = "PlatformIAMConfigured"

	PhaseInfrastructure = "Infrastructure"
	PhaseHostedCluster  = "HostedCluster"
	PhaseNodePools      = "NodePools"
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

	// HostedCluster that will be applied to the ManagementCluster by ACM, if omitted, it will be generated
	// +optional
	HostedClusterSpec *hypv1alpha1.HostedClusterSpec `json:"hostedClusterSpec,omitempty"`

	// NodePools is an array of NodePool resources that will be applied to the ManagementCluster by ACM,
	// if omitted, a default NodePool will be generated
	// +optional
	NodePools []*HypershiftNodePools `json:"nodePools,omitempty"`
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
	// +immutable
	CloudProvider corev1.LocalObjectReference `json:"cloudProvider"`
}

type Platforms struct {
	AWS *AWSPlatform `json:"aws,omitempty"`
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
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformInfrastructureConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="IAM Ready",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformIAMConfigured\")].status",description="Configured"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformIAMConfigured\")].reason",description="Reason"
// +kubebuilder:printcolumn:name="INFRA Ready",type="string",JSONPath=".status.conditions[?(@.type==\"PlatformInfrastructureConfigured\")].status",description="Configured"

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
