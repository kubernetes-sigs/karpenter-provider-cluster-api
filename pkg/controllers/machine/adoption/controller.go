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

package adoption

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/controllers/provisioning/scheduling"
	"sigs.k8s.io/karpenter/pkg/operator/injection"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	clusterapi "sigs.k8s.io/karpenter-provider-cluster-api/pkg/cloudprovider"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machine"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/providers/machinedeployment"
)

const (
	// AdoptedAnnotation marks a NodeClaim created for a Machine that Karpenter did
	// not create, such as a replacement Machine from a Cluster API rollout.
	AdoptedAnnotation = v1alpha1.Group + "/adopted"

	// GracePeriod is how long a Machine must exist before it can be adopted
	// outside a rollout. It is longer than the create batcher waits for new
	// Machines, so Machines that Karpenter is about to claim for its own
	// NodeClaims are left alone.
	GracePeriod = 2 * time.Minute

	// resyncPeriod is how often a MachineDeployment is reconciled without a
	// Machine event, to pick up NodePool and NodeClass changes.
	resyncPeriod = 5 * time.Minute
)

// Controller adopts the Machines that Cluster API creates in Karpenter's
// MachineDeployments, for NodeClasses that leave upgrades to Cluster API. During
// a rollout Cluster API replaces Karpenter's Machines with Machines that no
// NodeClaim references. Without a NodeClaim, Karpenter never registers their
// Nodes or scales them down. The controller creates a NodeClaim for each one,
// bound to the Machine through the Machine annotation, so Karpenter manages it
// like any other node.
//
// It reconciles a MachineDeployment whenever one of its Machines changes, so a
// replacement Machine is adopted as soon as it is running.
type Controller struct {
	kubeClient                client.Client
	machineProvider           machine.Provider
	machineDeploymentProvider machinedeployment.Provider
	managementCache           cache.Cache
	clock                     clock.Clock
}

func NewController(clk clock.Clock, kubeClient client.Client, machineProvider machine.Provider, machineDeploymentProvider machinedeployment.Provider, managementCache cache.Cache) *Controller {
	return &Controller{
		kubeClient:                kubeClient,
		machineProvider:           machineProvider,
		machineDeploymentProvider: machineDeploymentProvider,
		managementCache:           managementCache,
		clock:                     clk,
	}
}

func (c *Controller) Name() string {
	return "machine.adoption"
}

// Reconcile adopts the Machines of the MachineDeployment named by the request.
func (c *Controller) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, c.Name())

	md, err := c.machineDeploymentProvider.Get(ctx, req.Name, req.Namespace)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if _, member := md.GetLabels()[providers.NodePoolMemberLabel]; !member {
		return reconcile.Result{}, nil
	}

	nodePool, instanceType, err := c.nodePoolForMachineDeployment(ctx, md)
	if err != nil {
		return reconcile.Result{}, err
	}
	if nodePool == nil {
		return reconcile.Result{RequeueAfter: resyncPeriod}, nil
	}

	nodeClaimList := &karpv1.NodeClaimList{}
	if err := c.kubeClient.List(ctx, nodeClaimList); err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to list NodeClaims: %w", err)
	}
	// the Machines that NodeClaims are bound to, keyed by the Machine annotation value
	claimed := map[string]bool{}
	for _, nc := range nodeClaimList.Items {
		if machineRef, ok := nc.Annotations[providers.MachineAnnotation]; ok {
			claimed[machineRef] = true
		}
	}

	machines, err := c.machineProvider.List(ctx, md.Namespace, &metav1.LabelSelector{
		MatchLabels: map[string]string{capiv1beta1.MachineDeploymentNameLabel: md.Name},
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("unable to list Machines for MachineDeployment %q: %w", md.Name, err)
	}

	// While Cluster API replaces Machines they span more than one MachineSet, and
	// the Machines it creates are adopted without waiting out the grace period.
	machineSets := map[string]bool{}
	for _, m := range machines {
		machineSets[m.GetLabels()[capiv1beta1.MachineSetNameLabel]] = true
	}
	rollingOut := len(machineSets) > 1

	requeueAfter := resyncPeriod
	var errs []error
	for _, m := range machines {
		if claimed[fmt.Sprintf("%s/%s", m.Namespace, m.Name)] || !c.adoptable(m) {
			continue
		}
		if wait := GracePeriod - c.clock.Since(m.CreationTimestamp.Time); !rollingOut && wait > 0 {
			requeueAfter = min(requeueAfter, wait)
			continue
		}
		if err := c.adopt(ctx, nodePool, instanceType, m); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return reconcile.Result{}, fmt.Errorf("unable to adopt Machines of MachineDeployment %q: %v", md.Name, errs)
	}

	return reconcile.Result{RequeueAfter: requeueAfter}, nil
}

// nodePoolForMachineDeployment returns the NodePool that adopts the
// MachineDeployment's Machines, and the instance type it offers for them: the
// first NodePool by name whose NodeClass leaves upgrades to Cluster API, selects
// the MachineDeployment, and could place a NodeClaim on it.
func (c *Controller) nodePoolForMachineDeployment(ctx context.Context, md *capiv1beta1.MachineDeployment) (*karpv1.NodePool, *cloudprovider.InstanceType, error) {
	nodePoolList := &karpv1.NodePoolList{}
	if err := c.kubeClient.List(ctx, nodePoolList); err != nil {
		return nil, nil, fmt.Errorf("unable to list NodePools: %w", err)
	}
	sort.Slice(nodePoolList.Items, func(i, j int) bool {
		return nodePoolList.Items[i].Name < nodePoolList.Items[j].Name
	})

	instanceType := clusterapi.InstanceTypeForMachineDeployment(md)
	for i := range nodePoolList.Items {
		nodePool := &nodePoolList.Items[i]
		if !nodePool.DeletionTimestamp.IsZero() {
			continue
		}
		nodeClassRef := nodePool.Spec.Template.Spec.NodeClassRef
		if nodeClassRef == nil || nodeClassRef.Group != v1alpha1.Group || nodeClassRef.Kind != "ClusterAPINodeClass" {
			continue
		}
		nodeClass := &v1alpha1.ClusterAPINodeClass{}
		if err := c.kubeClient.Get(ctx, client.ObjectKey{Name: nodeClassRef.Name}, nodeClass); err != nil {
			if client.IgnoreNotFound(err) != nil {
				return nil, nil, fmt.Errorf("unable to get NodeClass %q: %w", nodeClassRef.Name, err)
			}
			continue
		}
		if nodeClass.UpgradeManager() != v1alpha1.UpgradeManagerClusterAPI {
			continue
		}
		selector := labels.Everything()
		if nodeClass.Spec.ScalableResourceSelector != nil {
			s, err := metav1.LabelSelectorAsSelector(nodeClass.Spec.ScalableResourceSelector)
			if err != nil {
				return nil, nil, fmt.Errorf("unable to convert selector of NodeClass %q: %w", nodeClass.Name, err)
			}
			selector = s
		}
		if !selector.Matches(labels.Set(md.GetLabels())) {
			continue
		}
		// the same check the scheduler makes before it places a NodeClaim on an instance type
		if instanceType.Requirements.Intersects(scheduling.NewNodeClaimTemplate(nodePool).Requirements) != nil {
			continue
		}
		return nodePool, instanceType, nil
	}
	return nil, nil, nil
}

// adoptable reports whether a Machine is running a Node that Karpenter has not
// claimed.
func (c *Controller) adoptable(m *capiv1beta1.Machine) bool {
	if c.machineProvider.IsDeleting(m) || m.Spec.ProviderID == nil {
		return false
	}
	if m.Status.GetTypedPhase() != capiv1beta1.MachinePhaseRunning {
		return false
	}
	if _, member := m.GetLabels()[providers.NodePoolMemberLabel]; member {
		return false
	}
	_, marked := m.GetAnnotations()[capiv1beta1.DeleteMachineAnnotation]
	return !marked
}

// adopt claims a Machine with the NodePool member label, so the create batcher
// does not also claim it, then creates a NodeClaim bound to it. Karpenter launches
// that NodeClaim through the cloud provider, which returns the existing Machine.
func (c *Controller) adopt(ctx context.Context, nodePool *karpv1.NodePool, instanceType *cloudprovider.InstanceType, m *capiv1beta1.Machine) error {
	labels := m.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels[providers.NodePoolMemberLabel] = ""
	m.SetLabels(labels)
	// the update fails on a conflict if the create batcher claimed the Machine first
	if err := c.machineProvider.Update(ctx, m); err != nil {
		return fmt.Errorf("unable to label Machine %q: %w", m.Name, err)
	}

	nodeClaim := nodeClaimForMachine(nodePool, instanceType, m)
	if err := c.kubeClient.Create(ctx, nodeClaim); err != nil && !errors.IsAlreadyExists(err) {
		delete(labels, providers.NodePoolMemberLabel)
		if rollbackErr := c.machineProvider.Update(ctx, m); rollbackErr != nil {
			log.FromContext(ctx).Error(rollbackErr, "unable to remove member label from Machine", "machine", m.Name)
		}
		return fmt.Errorf("unable to create NodeClaim for Machine %q: %w", m.Name, err)
	}
	log.FromContext(ctx).Info("adopted machine", "machine", m.Name, "nodeClaim", nodeClaim.Name, "nodePool", nodePool.Name)

	return nil
}

// nodeClaimForMachine builds the NodeClaim that the NodePool would have created
// for the Machine's instance type, bound to the Machine. Its name is derived from
// the Machine, so a repeated adoption finds the NodeClaim already there.
func nodeClaimForMachine(nodePool *karpv1.NodePool, instanceType *cloudprovider.InstanceType, m *capiv1beta1.Machine) *karpv1.NodeClaim {
	nodeClaimTemplate := scheduling.NewNodeClaimTemplate(nodePool)
	nodeClaimTemplate.Requirements.Add(instanceType.Requirements.Values()...)
	nodeClaimTemplate.InstanceTypeOptions = cloudprovider.InstanceTypes{instanceType}

	nodeClaim := nodeClaimTemplate.ToNodeClaim()
	nodeClaim.GenerateName = ""
	nodeClaim.Name = fmt.Sprintf("%s-%s", nodePool.Name, m.Name)
	nodeClaim.Annotations = lo.Assign(nodeClaim.Annotations, map[string]string{
		providers.MachineAnnotation: fmt.Sprintf("%s/%s", m.Namespace, m.Name),
		AdoptedAnnotation:           "true",
	})
	return nodeClaim
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(c.Name()).
		WatchesRawSource(source.Kind(c.managementCache, &capiv1beta1.Machine{},
			handler.TypedEnqueueRequestsFromMapFunc(machineDeploymentForMachine))).
		Complete(c)
}

// machineDeploymentForMachine maps a Machine event to its MachineDeployment.
func machineDeploymentForMachine(_ context.Context, m *capiv1beta1.Machine) []reconcile.Request {
	name, found := m.GetLabels()[capiv1beta1.MachineDeploymentNameLabel]
	if !found {
		return nil
	}
	return []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: m.Namespace, Name: name}}}
}
