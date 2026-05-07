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

package capi

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/samber/lo"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/karpenter/pkg/test"
	"sigs.k8s.io/karpenter/test/pkg/environment/common"

	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
)

var managementKubeconfig = flag.String("management-kubeconfig", "", "kubeconfig for the CAPI management cluster; falls back to CAPI_MANAGEMENT_KUBECONFIG env, then ~/.kube/config")

func init() {
	lo.Must0(v1alpha1.AddToScheme(scheme.Scheme))
	test.SetDefaultNodeClassType(&v1alpha1.ClusterAPINodeClass{})
}

type Environment struct {
	*common.Environment
	MgmtClient client.Client
}

func NewEnvironment(t *testing.T) *Environment {
	env := common.NewEnvironment(t)
	mgmtClient, err := newManagementClusterClient()
	Expect(err).NotTo(HaveOccurred(), "failed to create management cluster client")
	return &Environment{
		Environment: env,
		MgmtClient:  mgmtClient,
	}
}

func ManagementKubeconfigPath() string {
	if p := *managementKubeconfig; p != "" {
		return p
	}
	if p := os.Getenv("CAPI_MANAGEMENT_KUBECONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	return filepath.Join(home, ".kube", "config")
}

func newManagementClusterClient() (client.Client, error) {
	path := ManagementKubeconfigPath()
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load management kubeconfig %q: %w", path, err)
	}
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = capiv1beta1.AddToScheme(s)
	return client.New(cfg, client.Options{Scheme: s})
}
