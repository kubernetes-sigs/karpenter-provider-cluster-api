/*
Copyright 2025 The Kubernetes Authors.

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

package status_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	awsstatus "github.com/awslabs/operatorpkg/status"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"
)

var _ = Describe("NodeClass Status Controller", func() {
	AfterEach(func() {
		test.EventuallyDeleteAllOf(cl, &v1alpha1.ClusterAPINodeClass{}, &v1alpha1.ClusterAPINodeClassList{}, testNamespace)
	})

	It("adds the ready condition to a new NodeClass", func() {
		nodeClass := &v1alpha1.ClusterAPINodeClass{}
		nodeClass.Name = "default"
		ExpectApplied(ctx, cl, nodeClass)
		ExpectObjectReconciled(ctx, cl, controller, nodeClass)
		nodeClass = ExpectExists(ctx, cl, nodeClass)

		Expect(nodeClass.StatusConditions().IsTrue(awsstatus.ConditionReady)).To(BeTrue())
	})
})
