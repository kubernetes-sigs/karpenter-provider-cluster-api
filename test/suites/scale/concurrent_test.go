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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	coretest "sigs.k8s.io/karpenter/pkg/test"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
)

// onePerNodeDeployment creates a deployment whose pods are forced onto separate
// nodes via a hard pod anti-affinity on hostname. The anti-affinity is set
// directly on the deployment spec rather than through PodOptions because
// mergo.Merge in the upstream Deployment factory drops slice fields.
func onePerNodeDeployment(replicas int32) *appsv1.Deployment {
	dep := coretest.Deployment(coretest.DeploymentOptions{
		Replicas: replicas,
		PodOptions: coretest.PodOptions{
			ObjectMeta: metav1.ObjectMeta{
				Labels: testLabels,
			},
			ResourceRequirements: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
			},
		},
	})
	dep.Spec.Template.Spec.Affinity = &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
				TopologyKey:   corev1.LabelHostname,
				LabelSelector: dep.Spec.Selector,
			}},
		},
	}
	return dep
}

var _ = Describe("Concurrent create and delete", func() {
	const (
		provisionTimeout   = 3 * time.Minute
		convergenceTimeout = 5 * time.Minute
	)

	It("should converge to 8 replicas when scaling up immediately after scaling down", func() {
		deployment := onePerNodeDeployment(5)
		env.ExpectCreated(deployment)

		nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("0s")
		env.ExpectCreated(nodePool, nodeClass)

		env.EventuallyExpectHealthyPodCountWithTimeout(provisionTimeout, labelSelector, 5)
		eventuallyExpectKarpenterLabeledNodeCount(5, provisionTimeout)

		scaleDeployment(deployment, 2)
		scaleDeployment(deployment, 8)

		env.EventuallyExpectHealthyPodCountWithTimeout(convergenceTimeout, labelSelector, 8)
		eventuallyExpectKarpenterLabeledNodeCount(8, convergenceTimeout)
		eventuallyExpectKarpenterLabeledNodeClaimCount(8, convergenceTimeout)
		eventuallyExpectKarpenterMachineDeploymentReplicas(8, convergenceTimeout)
	})

	It("should converge to 5 replicas after rapid sequential scale changes", func() {
		deployment := onePerNodeDeployment(3)
		env.ExpectCreated(deployment)

		nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("0s")
		env.ExpectCreated(nodePool, nodeClass)

		env.EventuallyExpectHealthyPodCountWithTimeout(provisionTimeout, labelSelector, 3)
		eventuallyExpectKarpenterLabeledNodeCount(3, provisionTimeout)

		scaleDeployment(deployment, 7)
		scaleDeployment(deployment, 2)
		scaleDeployment(deployment, 10)
		scaleDeployment(deployment, 1)
		scaleDeployment(deployment, 5)

		env.EventuallyExpectHealthyPodCountWithTimeout(convergenceTimeout, labelSelector, 5)
		eventuallyExpectKarpenterLabeledNodeCount(5, convergenceTimeout)
		eventuallyExpectKarpenterLabeledNodeClaimCount(5, convergenceTimeout)
		eventuallyExpectKarpenterMachineDeploymentReplicas(5, convergenceTimeout)
	})

	It("should converge to 5 replicas after multiple separate batch cycles", func() {
		deployment := onePerNodeDeployment(3)
		env.ExpectCreated(deployment)

		nodePool.Spec.Disruption.ConsolidateAfter = karpv1.MustParseNillableDuration("0s")
		env.ExpectCreated(nodePool, nodeClass)

		env.EventuallyExpectHealthyPodCountWithTimeout(provisionTimeout, labelSelector, 3)
		eventuallyExpectKarpenterLabeledNodeCount(3, provisionTimeout)

		scaleDeployment(deployment, 7)
		eventuallyExpectKarpenterMachineDeploymentReplicas(7, provisionTimeout)

		scaleDeployment(deployment, 2)
		eventuallyExpectKarpenterMachineDeploymentReplicas(2, convergenceTimeout)

		scaleDeployment(deployment, 10)
		eventuallyExpectKarpenterMachineDeploymentReplicas(10, provisionTimeout)

		scaleDeployment(deployment, 1)
		eventuallyExpectKarpenterMachineDeploymentReplicas(1, convergenceTimeout)

		scaleDeployment(deployment, 5)

		env.EventuallyExpectHealthyPodCountWithTimeout(convergenceTimeout, labelSelector, 5)
		eventuallyExpectKarpenterLabeledNodeCount(5, convergenceTimeout)
		eventuallyExpectKarpenterLabeledNodeClaimCount(5, convergenceTimeout)
		eventuallyExpectKarpenterMachineDeploymentReplicas(5, convergenceTimeout)
	})
})

func scaleDeployment(dep *appsv1.Deployment, replicas int32) {
	GinkgoHelper()
	current := &appsv1.Deployment{}
	Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(dep), current)).To(Succeed())
	current.Spec.Replicas = lo.ToPtr(replicas)
	env.ExpectUpdated(current)
}

func eventuallyExpectKarpenterLabeledNodeCount(count int, timeout time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		nodeList := &corev1.NodeList{}
		g.Expect(env.Client.List(env.Context, nodeList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeList.Items)).To(Equal(count),
			fmt.Sprintf("expected workload Node count == %d, got %d", count, len(nodeList.Items)))
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
}

func eventuallyExpectKarpenterLabeledNodeClaimCount(count int, timeout time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		nodeClaimList := &karpv1.NodeClaimList{}
		g.Expect(env.Client.List(env.Context, nodeClaimList, client.HasLabels{coretest.DiscoveryLabel})).To(Succeed())
		g.Expect(len(nodeClaimList.Items)).To(Equal(count))
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
}

func eventuallyExpectKarpenterMachineDeploymentReplicas(want int32, timeout time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		mdList := &capiv1beta1.MachineDeploymentList{}
		g.Expect(env.MgmtClient.List(env.Context, mdList,
			client.MatchingLabels{providers.NodePoolMemberLabel: ""})).To(Succeed())
		g.Expect(mdList.Items).NotTo(BeEmpty())

		var total int32
		namespaced := make([]string, 0, len(mdList.Items))
		for i := range mdList.Items {
			total += lo.FromPtr(mdList.Items[i].Spec.Replicas)
			namespaced = append(namespaced, mdList.Items[i].Namespace+"/"+mdList.Items[i].Name)
		}
		g.Expect(total).To(Equal(want),
			fmt.Sprintf("expected sum(MachineDeployment.spec.replicas) == %d, got %d for %v", want, total, namespaced))
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
}
