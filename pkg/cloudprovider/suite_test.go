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

package cloudprovider

import (
	"context"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2/textlogger"
	capiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/karpenter-provider-cluster-api/pkg/apis/v1alpha1"
	karpv1beta1 "sigs.k8s.io/karpenter/pkg/apis/v1beta1"
)

const (
	testNamespace = "karpenter-cluster-api"
)

var cfg *rest.Config
var cl client.Client
var testEnv *envtest.Environment
var testScheme *runtime.Scheme

func TestMachineProvider(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "CloudProvider Suite")
}

var _ = BeforeSuite(func() {
	var err error
	logf.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "vendor", "sigs.k8s.io", "cluster-api", "api", "v1beta1"),
			filepath.Join("..", "apis", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	testScheme = scheme.Scheme
	Expect(capiv1beta1.AddToScheme(testScheme)).To(Succeed())
	Expect(v1alpha1.AddToScheme(testScheme)).To(Succeed())
	Expect(karpv1beta1.SchemeBuilder.AddToScheme(testScheme)).To(Succeed())

	cl, err = client.New(cfg, client.Options{Scheme: testScheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(cl).NotTo(BeNil())

	namespace := &corev1.Namespace{}
	namespace.SetName(testNamespace)
	Expect(cl.Create(context.Background(), namespace)).To(Succeed())
})

var _ = AfterSuite(func() {
	By("tearing down the test environment")
	err := testEnv.Stop()
	Expect(err).NotTo(HaveOccurred())
})
