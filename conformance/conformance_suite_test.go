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

package conformance

import (
	"flag"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clusterclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

type clusterClients struct {
	k8s              kubernetes.Interface
	clusterInventory clusterclient.Interface
	rest             *rest.Config
}

var client clusterClients
var loadingRules *clientcmd.ClientConfigLoadingRules

func TestConformance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Conformance Suite")
}

func init() {
	loadingRules = clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	flag.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "", "absolute path(s) to the kubeconfig file(s)")
}

var _ = BeforeSuite(func() {
	Expect(setupClient()).To(Succeed())
})

func setupClient() error {
	overrides := clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &overrides)
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return err
	}
	k8sClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	cluenterInventoryClient, err := clusterclient.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	client = clusterClients{k8s: k8sClient, clusterInventory: cluenterInventoryClient, rest: restConfig}
	return nil
}
