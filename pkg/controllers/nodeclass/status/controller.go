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

package status

import (
	"context"

	"github.com/awslabs/operatorpkg/status"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	"sigs.k8s.io/karpenter/pkg/operator/injection"
)

type Controller struct {
	kubeClient client.Client
}

func NewController(kubeClient client.Client) *Controller {
	return &Controller{
		kubeClient: kubeClient,
	}
}

func (c *Controller) Reconcile(ctx context.Context, nodeClass *v1alpha1.ClusterAPINodeClass) (reconcile.Result, error) {
	ctx = injection.WithControllerName(ctx, "nodeclass.status")
	stored := nodeClass.DeepCopy()

	// for now, we just want to set all nodeclasses to ready. in the future we may want
	// to add other types of checks to ensure that the underlying cluster-api objects are
	// also ready.
	ready := nodeClass.StatusConditions().Get(status.ConditionReady)
	if ready.IsUnknown() || ready.IsFalse() {
		nodeClass.StatusConditions().SetTrue(status.ConditionReady)
	}

	if !equality.Semantic.DeepEqual(stored, nodeClass) {
		// this code is inspired by karpenter/pkg/controllers/nodepool/readiness controller
		if err := c.kubeClient.Status().Patch(ctx, nodeClass, client.MergeFromWithOptions(stored, client.MergeFromWithOptimisticLock{})); client.IgnoreNotFound(err) != nil {
			if errors.IsConflict(err) {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	b := controllerruntime.NewControllerManagedBy(m).
		Named("nodeclass.status").
		For(&v1alpha1.ClusterAPINodeClass{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 10})

	return b.Complete(reconcile.AsReconciler(m.GetClient(), c))
}
