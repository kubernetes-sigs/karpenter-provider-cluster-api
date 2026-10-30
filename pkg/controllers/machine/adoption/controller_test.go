/*
Copyright 2026 The Kubernetes Authors.

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

package adoption_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	karptest "sigs.k8s.io/karpenter/pkg/test"
	. "sigs.k8s.io/karpenter/pkg/test/expectations"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/controllers/machine/adoption"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/test"
)

var _ = Describe("Machine Adoption Controller", func() {
	var nodeClass *v1alpha1.ClusterAPINodeClass
	var nodePool *karpv1.NodePool
	var machineDeployment *capiv1beta1.MachineDeployment

	BeforeEach(func() {
		// the Machines created below are older than the grace period
		fakeClock.SetTime(time.Now().Add(2 * adoption.GracePeriod))

		nodeClass = &v1alpha1.ClusterAPINodeClass{}
		nodeClass.Name = "default"
		nodeClass.Spec.Upgrades = &v1alpha1.UpgradesSpec{ManagedBy: v1alpha1.UpgradeManagerClusterAPI}
		ExpectApplied(ctx, cl, nodeClass)

		nodePool = karptest.NodePool(karpv1.NodePool{
			ObjectMeta: metav1.ObjectMeta{Name: "pool"},
			Spec: karpv1.NodePoolSpec{
				Template: karpv1.NodeClaimTemplate{
					Spec: karpv1.NodeClaimTemplateSpec{
						NodeClassRef: &karpv1.NodeClassReference{
							Group: v1alpha1.Group,
							Kind:  "ClusterAPINodeClass",
							Name:  nodeClass.Name,
						},
					},
				},
			},
		})
		ExpectApplied(ctx, cl, nodePool)

		machineDeployment = newMachineDeployment("md-1")
		ExpectApplied(ctx, cl, machineDeployment)
	})

	AfterEach(func() {
		test.EventuallyDeleteAllOf(cl, &capiv1beta1.Machine{}, &capiv1beta1.MachineList{}, testNamespace)
		test.EventuallyDeleteAllOf(cl, &capiv1beta1.MachineDeployment{}, &capiv1beta1.MachineDeploymentList{}, testNamespace)
		test.EventuallyDeleteAllOf(cl, &karpv1.NodeClaim{}, &karpv1.NodeClaimList{}, "")
		test.EventuallyDeleteAllOf(cl, &karpv1.NodePool{}, &karpv1.NodePoolList{}, "")
		test.EventuallyDeleteAllOf(cl, &v1alpha1.ClusterAPINodeClass{}, &v1alpha1.ClusterAPINodeClassList{}, "")
	})

	It("adopts a running Machine that no NodeClaim is bound to", func() {
		m := createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		nodeClaim := &karpv1.NodeClaim{}
		Expect(cl.Get(ctx, client.ObjectKey{Name: "pool-m-1"}, nodeClaim)).To(Succeed())
		Expect(nodeClaim.Annotations).To(HaveKeyWithValue(providers.MachineAnnotation, testNamespace+"/m-1"))
		Expect(nodeClaim.Annotations).To(HaveKeyWithValue(adoption.AdoptedAnnotation, "true"))
		Expect(nodeClaim.Annotations).To(HaveKeyWithValue(karpv1.NodePoolHashAnnotationKey, nodePool.Hash()))
		Expect(nodeClaim.Labels).To(HaveKeyWithValue(karpv1.NodePoolLabelKey, nodePool.Name))
		Expect(nodeClaim.Spec.Requirements).To(ContainElement(HaveField("NodeSelectorRequirement", corev1.NodeSelectorRequirement{
			Key:      "node-role.kubernetes.io/worker",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{""},
		})))
		Expect(nodeClaim.Spec.NodeClassRef.Name).To(Equal(nodeClass.Name))
		Expect(nodeClaim.OwnerReferences).To(HaveLen(1))
		Expect(nodeClaim.OwnerReferences[0].Name).To(Equal(nodePool.Name))

		m = ExpectExists(ctx, cl, m)
		Expect(m.Labels).To(HaveKey(providers.NodePoolMemberLabel))
	})

	It("adopts a Machine only once", func() {
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)
		reconcileMachineDeployment(machineDeployment)

		nodeClaimList := &karpv1.NodeClaimList{}
		Expect(cl.List(ctx, nodeClaimList)).To(Succeed())
		Expect(nodeClaimList.Items).To(HaveLen(1))
	})

	It("leaves Machines alone when the NodeClass leaves upgrades to Karpenter", func() {
		nodeClass.Spec.Upgrades.ManagedBy = v1alpha1.UpgradeManagerKarpenter
		ExpectApplied(ctx, cl, nodeClass)
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("adopts a young Machine while Cluster API replaces Machines", func() {
		fakeClock.SetTime(time.Now())
		old := newMachine("m-old", machineDeployment)
		old.Labels[capiv1beta1.MachineSetNameLabel] = machineDeployment.Name + "-old"
		old.Labels[providers.NodePoolMemberLabel] = ""
		ExpectApplied(ctx, cl, old)
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		nodeClaim := &karpv1.NodeClaim{}
		Expect(cl.Get(ctx, client.ObjectKey{Name: "pool-m-1"}, nodeClaim)).To(Succeed())
	})

	It("requeues a young Machine for when its grace period ends", func() {
		fakeClock.SetTime(time.Now())
		createRunningMachine("m-1", machineDeployment)

		result := reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))
		Expect(result.RequeueAfter).To(BeNumerically("<=", adoption.GracePeriod))
	})

	It("leaves MachineDeployments without the member label alone", func() {
		delete(machineDeployment.Labels, providers.NodePoolMemberLabel)
		ExpectApplied(ctx, cl, machineDeployment)
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("leaves MachineDeployments the NodeClass does not select alone", func() {
		nodeClass.Spec.ScalableResourceSelector = &metav1.LabelSelector{MatchLabels: map[string]string{"pool": "other"}}
		ExpectApplied(ctx, cl, nodeClass)
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("leaves Machines younger than the grace period alone", func() {
		fakeClock.SetTime(time.Now())
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("leaves Machines that are not running alone", func() {
		m := newMachine("m-1", machineDeployment)
		ExpectApplied(ctx, cl, m)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("leaves Machines that Karpenter claimed alone", func() {
		m := newMachine("m-1", machineDeployment)
		m.Labels[providers.NodePoolMemberLabel] = ""
		ExpectApplied(ctx, cl, m)
		setRunning(m)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})

	It("leaves Machines that a NodeClaim is bound to alone", func() {
		createRunningMachine("m-1", machineDeployment)
		nodeClaim := karptest.NodeClaim(karpv1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "existing",
				Annotations: map[string]string{providers.MachineAnnotation: testNamespace + "/m-1"},
			},
			Spec: karpv1.NodeClaimSpec{NodeClassRef: nodePool.Spec.Template.Spec.NodeClassRef},
		})
		ExpectApplied(ctx, cl, nodeClaim)

		reconcileMachineDeployment(machineDeployment)

		nodeClaimList := &karpv1.NodeClaimList{}
		Expect(cl.List(ctx, nodeClaimList)).To(Succeed())
		Expect(nodeClaimList.Items).To(HaveLen(1))
		Expect(nodeClaimList.Items[0].Name).To(Equal("existing"))
	})

	It("leaves Machines alone when the NodePool cannot use their MachineDeployment", func() {
		nodePool.Spec.Template.Spec.Requirements = []karpv1.NodeSelectorRequirementWithMinValues{{
			NodeSelectorRequirement: corev1.NodeSelectorRequirement{
				Key:      "node-role.kubernetes.io/worker",
				Operator: corev1.NodeSelectorOpDoesNotExist,
			},
		}}
		ExpectApplied(ctx, cl, nodePool)
		createRunningMachine("m-1", machineDeployment)

		reconcileMachineDeployment(machineDeployment)

		expectNoNodeClaims()
	})
})

func reconcileMachineDeployment(md *capiv1beta1.MachineDeployment) reconcile.Result {
	GinkgoHelper()
	result, err := controller.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(md)})
	Expect(err).ToNot(HaveOccurred())
	return result
}

func expectNoNodeClaims() {
	GinkgoHelper()
	nodeClaimList := &karpv1.NodeClaimList{}
	Expect(cl.List(ctx, nodeClaimList)).To(Succeed())
	Expect(nodeClaimList.Items).To(BeEmpty())
}

func newMachineDeployment(name string) *capiv1beta1.MachineDeployment {
	md := &capiv1beta1.MachineDeployment{}
	md.Name = name
	md.Namespace = testNamespace
	md.Labels = map[string]string{providers.NodePoolMemberLabel: ""}
	md.Annotations = map[string]string{
		"capacity.cluster-autoscaler.kubernetes.io/cpu":    "4",
		"capacity.cluster-autoscaler.kubernetes.io/memory": "8G",
	}
	md.Spec.ClusterName = "test-cluster"
	md.Spec.Replicas = ptr.To(int32(1))
	md.Spec.Template.Spec.ClusterName = "test-cluster"
	md.Spec.Template.Labels = map[string]string{"node-role.kubernetes.io/worker": ""}
	return md
}

func newMachine(name string, md *capiv1beta1.MachineDeployment) *capiv1beta1.Machine {
	m := &capiv1beta1.Machine{}
	m.Name = name
	m.Namespace = testNamespace
	m.Labels = map[string]string{
		capiv1beta1.MachineDeploymentNameLabel: md.Name,
		capiv1beta1.MachineSetNameLabel:        md.Name + "-new",
	}
	m.Spec.ClusterName = md.Spec.ClusterName
	m.Spec.ProviderID = ptr.To(fmt.Sprintf("clusterapi://%s", name))
	return m
}

func setRunning(m *capiv1beta1.Machine) {
	GinkgoHelper()
	m = ExpectExists(ctx, cl, m)
	m.Status.SetTypedPhase(capiv1beta1.MachinePhaseRunning)
	Expect(cl.Status().Update(ctx, m)).To(Succeed())
}

func createRunningMachine(name string, md *capiv1beta1.MachineDeployment) *capiv1beta1.Machine {
	GinkgoHelper()
	m := newMachine(name, md)
	ExpectApplied(ctx, cl, m)
	setRunning(m)
	return m
}
