/*
Copyright The Kubernetes Authors.

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
}

// ClusterAPINodeClassStatus is the status for ClusterAPINodeClasses
type ClusterAPINodeClassStatus struct{}

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
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Items []ClusterAPINodeClass `json:"items"`
}
