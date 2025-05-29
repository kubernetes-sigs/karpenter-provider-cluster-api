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

package test

import (
	"context"

	. "github.com/onsi/gomega"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func EventuallyDeleteAllOf(cl client.Client, obj client.Object, ls client.ObjectList, namespace string) {
	Expect(cl.DeleteAllOf(context.Background(), obj, client.InNamespace(namespace))).To(Succeed())
	Eventually(func() client.ObjectList {
		Expect(cl.List(context.Background(), ls, client.InNamespace(namespace))).To(Succeed())
		return ls
	}).Should(HaveField("Items", HaveLen(0)))
}
