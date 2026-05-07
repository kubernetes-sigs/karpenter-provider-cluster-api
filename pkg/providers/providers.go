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

package providers

import (
	"fmt"
	"strings"
)

const (
	// NodePoolMemberLabel is a label used to identify resources that are used by karpenter
	NodePoolMemberLabel = "node.cluster.x-k8s.io/karpenter-member"

	// MachineAnnotation is the annotation on a NodeClaim that references
	// the CAPI Machine bound to it, in "namespace/name" format.
	MachineAnnotation = "cluster.x-k8s.io/machine"
)

// ParseMachineAnnotation splits a "namespace/name" annotation value into its components.
func ParseMachineAnnotation(annotationValue string) (string, string, error) {
	parts := strings.Split(annotationValue, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid machine annotation %q: expected 'namespace/name'", annotationValue)
	}

	ns := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])

	if ns == "" || name == "" {
		return "", "", fmt.Errorf("invalid machine annotation %q: namespace and name cannot be empty", annotationValue)
	}

	return ns, name, nil
}
