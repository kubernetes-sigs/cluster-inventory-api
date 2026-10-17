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

// conformance-envtest starts an envtest control plane with the repository's
// CRDs installed and writes an admin kubeconfig for it, so the conformance
// suite can run against a baseline API server without a real cluster.
// Run from the repository root:
//
//	KUBEBUILDER_ASSETS=$(bin/setup-envtest-release-0.23 use 1.35.0 --bin-dir bin -p path) \
//	    go run ./hack/conformance-envtest --kubeconfig-out envtest.kubeconfig
//
// The control plane runs until the process receives SIGINT or SIGTERM.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func main() {
	var kubeconfigOut string

	flag.StringVar(&kubeconfigOut, "kubeconfig-out", "envtest.kubeconfig",
		"path to write the kubeconfig for the envtest control plane")
	flag.Parse()

	if err := run(kubeconfigOut); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(kubeconfigOut string) error {
	testEnv := &envtest.Environment{
		ErrorIfCRDPathMissing: true,
		CRDDirectoryPaths: []string{
			filepath.Join("config", "crd", "bases"),
		},
	}

	cfg, err := testEnv.Start()
	if err != nil {
		return fmt.Errorf("error starting the envtest control plane: %w", err)
	}

	defer func() {
		if err := testEnv.Stop(); err != nil {
			fmt.Fprintln(os.Stderr, "Error stopping the envtest control plane:", err)
		}
	}()

	kubeconfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"envtest": {Server: cfg.Host, CertificateAuthorityData: cfg.CAData},
		},
		AuthInfos: map[string]*clientcmdapi.AuthInfo{
			"envtest": {
				ClientCertificateData: cfg.CertData,
				ClientKeyData:         cfg.KeyData,
				Token:                 cfg.BearerToken,
			},
		},
		Contexts: map[string]*clientcmdapi.Context{
			"envtest": {Cluster: "envtest", AuthInfo: "envtest"},
		},
		CurrentContext: "envtest",
	}

	if err := clientcmd.WriteToFile(kubeconfig, kubeconfigOut); err != nil {
		return fmt.Errorf("error writing the kubeconfig to %s: %w", kubeconfigOut, err)
	}

	fmt.Printf("envtest control plane running at %s, kubeconfig written to %s\n", cfg.Host, kubeconfigOut)

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	<-signalCh

	return nil
}
