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

package scale_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/test"

	capienv "sigs.k8s.io/karpenter-provider-cluster-api/test/pkg/environment/capi"
)

var (
	env           *capienv.Environment
	nodePool      *karpv1.NodePool
	nodeClass     *unstructured.Unstructured
	testLabels    = map[string]string{test.DiscoveryLabel: "owned"}
	labelSelector = labels.SelectorFromSet(testLabels)
)

func TestScale(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = capienv.NewEnvironment(t)
	})
	AfterSuite(func() {
		env.Stop()
	})
	RunSpecs(t, "Scale")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultNodeClass.DeepCopy()
	nodePool = env.DefaultNodePool(nodeClass)
	nodePool.Spec.Limits = karpv1.Limits{}
})

var _ = AfterEach(func() {
	env.Cleanup()
	env.AfterEach()
})
