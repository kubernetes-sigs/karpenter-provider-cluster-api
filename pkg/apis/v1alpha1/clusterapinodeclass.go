/*
Copyright 2024 The Kubernetes Authors.

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
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterAPINodeClassSpec is the top level specification for ClusterAPINodeClasses.
type ClusterAPINodeClassSpec struct {
	// scalableResourceSelector is a LabelSelector that is used to identify the Cluster API scalable
	// resources that are participating in Karpenter provisioning. For a deeper discussion of
	// how label selectors are used in Kubernetes, please see the following:
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/
	// https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/label-selector/
	ScalableResourceSelector *metav1.LabelSelector `json:"scalableResourceSelector,omitempty"`

	// upgrades configures how nodes are replaced when the Machine behind a node
	// no longer matches its MachineDeployment, for example after a Kubernetes
	// version bump or a template change.
	// +optional
	Upgrades *UpgradesSpec `json:"upgrades,omitempty"`
}

// UpgradeManager names the controller that replaces nodes whose Machine no
// longer matches its MachineDeployment.
// +kubebuilder:validation:Enum=ClusterAPI;Karpenter
type UpgradeManager string

const (
	// UpgradeManagerClusterAPI leaves upgrades to Cluster API. Karpenter does not
	// report drift for MachineDeployment changes, and the provider adopts the
	// Machines that Cluster API creates when it replaces Karpenter's during a
	// rollout, so Karpenter keeps managing the pool. To also stop Karpenter
	// replacing nodes for its own drift reasons, add a NodePool disruption
	// budget with reasons [Drifted] and nodes "0".
	UpgradeManagerClusterAPI UpgradeManager = "ClusterAPI"

	// UpgradeManagerKarpenter makes Karpenter replace nodes: a NodeClaim whose
	// Machine belongs to a MachineSet other than its MachineDeployment's current
	// one is drifted, and Karpenter replaces it within its disruption budgets.
	// The MachineDeployment's rollout strategy should be OnDelete, so Cluster
	// API does not replace the same Machines itself.
	UpgradeManagerKarpenter UpgradeManager = "Karpenter"
)

// UpgradesSpec configures node upgrades for a ClusterAPINodeClass.
type UpgradesSpec struct {
	// managedBy is the controller that replaces nodes whose Machine no longer
	// matches its MachineDeployment: ClusterAPI (the default) or Karpenter.
	// +kubebuilder:default=ClusterAPI
	// +optional
	ManagedBy UpgradeManager `json:"managedBy,omitempty"`
}

// UpgradeManager returns who replaces outdated nodes for this NodeClass,
// ClusterAPI when it is not set.
func (nc *ClusterAPINodeClass) UpgradeManager() UpgradeManager {
	if nc.Spec.Upgrades == nil || nc.Spec.Upgrades.ManagedBy == "" {
		return UpgradeManagerClusterAPI
	}
	return nc.Spec.Upgrades.ManagedBy
}

// ClusterAPINodeClassStatus is the status for ClusterAPINodeClasses
type ClusterAPINodeClassStatus struct {
	// Conditions contains signals for health and readiness
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`
}

// ClusterAPINodeClass is the Schema for the ClusterAPINodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=clusterapinodeclasses,scope=Cluster,categories=karpenter,shortName={capinc,capincs}
// +kubebuilder:subresource:status
type ClusterAPINodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterAPINodeClassSpec   `json:"spec,omitempty"`
	Status ClusterAPINodeClassStatus `json:"status,omitempty"`
}

// ClusterAPINodeClassList contains a list of ClusterAPINodeClasses
// +kubebuilder:object:root=true
type ClusterAPINodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ClusterAPINodeClass `json:"items"`
}

// The following methods are implemented to satisfy the Object interface
// from https://github.com/awslabs/operatorpkg/blob/main/status/condition.go
// which in turn is utilized by the cloudprovider.GetSupportedNodeClasses method.
func (nc *ClusterAPINodeClass) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions().For(nc)
}

func (nc *ClusterAPINodeClass) GetConditions() []status.Condition {
	return nc.Status.Conditions
}

func (nc *ClusterAPINodeClass) SetConditions(conditions []status.Condition) {
	nc.Status.Conditions = conditions
}
