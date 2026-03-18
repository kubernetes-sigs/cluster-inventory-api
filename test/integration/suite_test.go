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

package integration

import (
	"context"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

var testEnv *envtest.Environment
var testNamespace string
var cfg *rest.Config
var kubernetesClient kubernetes.Interface
var clusterProfileClient cpclientset.Interface

func TestIntegration(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "API Validation Integration Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	ginkgo.By("bootstrapping test environment")

	// start a kube-apiserver
	testEnv = &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join("../../", "config", "crd", "bases"),
		},
	}

	var err error
	cfg, err = testEnv.Start()
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
	gomega.Expect(cfg).ToNot(gomega.BeNil())

	clusterProfileClient, err = cpclientset.NewForConfig(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	kubernetesClient, err = kubernetes.NewForConfig(cfg)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	testNamespace = "cluster-profile-api-test-" + rand.String(5)
	_, err = kubernetesClient.CoreV1().Namespaces().
		Create(context.TODO(), &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}, metav1.CreateOptions{})
	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})

var _ = ginkgo.AfterSuite(func() {
	ginkgo.By("tearing down the test environment")

	// Skip if client wasn't instantiated
	if kubernetesClient != nil {
		err := kubernetesClient.CoreV1().Namespaces().
			Delete(context.TODO(), testNamespace, metav1.DeleteOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())

		err = testEnv.Stop()
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	}
})
