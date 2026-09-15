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

// conformance-seed populates an inventory namespace with ClusterProfile
// objects shaped like those of a conformant cluster manager. The conformance
// suite only observes implementation-managed objects, so this seeder stands in
// for a real implementation when exercising the suite itself, e.g. against an
// envtest control plane or a kind cluster in CI. Run from the repository root:
//
//	go run ./hack/conformance-seed --kubeconfig envtest.kubeconfig \
//	    --namespace conformance-inventory --cluster-manager conformance-sample-manager
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	cpv1alpha2 "sigs.k8s.io/cluster-inventory-api/apis/v1alpha2"
	cpclientset "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

func main() {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()

	var kubeContext, namespace, clusterManager string
	var clusters int

	flag.StringVar(&loadingRules.ExplicitPath, "kubeconfig", "",
		"path to the kubeconfig file of the cluster to seed")
	flag.StringVar(&kubeContext, "context", "", "kubeconfig context of the cluster to seed")
	flag.StringVar(&namespace, "namespace", "conformance-inventory",
		"inventory namespace to create and populate")
	flag.StringVar(&clusterManager, "cluster-manager", "conformance-sample-manager",
		"cluster manager name recorded in the seeded ClusterProfile objects")
	flag.IntVar(&clusters, "clusters", 2, "number of member clusters to seed")
	flag.Parse()

	if err := run(loadingRules, kubeContext, namespace, clusterManager, clusters); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(loadingRules *clientcmd.ClientConfigLoadingRules, kubeContext, namespace, clusterManager string,
	clusters int) error {
	ctx := context.Background()

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.CurrentContext = kubeContext

	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return fmt.Errorf("error building the Kubernetes client configuration: %w", err)
	}

	kubernetesClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("error creating the Kubernetes client: %w", err)
	}

	clusterProfileClient, err := cpclientset.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("error creating the ClusterProfile client: %w", err)
	}

	_, err = kubernetesClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("error creating the inventory namespace %q: %w", namespace, err)
	}

	for i := 1; i <= clusters; i++ {
		if err := seedClusterProfile(ctx, clusterProfileClient, namespace, clusterManager, i); err != nil {
			return err
		}
	}

	fmt.Printf("seeded %d ClusterProfile object(s) for cluster manager %q in namespace %q\n",
		clusters, clusterManager, namespace)

	return nil
}

func seedClusterProfile(ctx context.Context, client cpclientset.Interface, namespace, clusterManager string,
	index int) error {
	name := fmt.Sprintf("%s-cluster-%d", clusterManager, index)

	profile := &cpv1alpha2.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				cpv1alpha2.LabelClusterManagerKey:    clusterManager,
				cpv1alpha2.LabelInventoryMemberIDKey: name + "-id",
			},
		},
		Spec: cpv1alpha2.ClusterProfileSpec{
			DisplayName: name,
			ClusterManager: cpv1alpha2.ClusterManager{
				Name: clusterManager,
			},
		},
	}

	created, err := client.ApisV1alpha2().ClusterProfiles(namespace).Create(ctx, profile, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		created, err = client.ApisV1alpha2().ClusterProfiles(namespace).Get(ctx, name, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("error creating ClusterProfile %q: %w", name, err)
	}

	created.Status.Version.Kubernetes = "v1.35.0"
	created.Status.Properties = []cpv1alpha2.Property{{
		Name:             "location",
		Value:            fmt.Sprintf("region-%d", index),
		LastObservedTime: metav1.Now(),
	}}
	created.Status.AccessProviders = []cpv1alpha2.AccessProvider{{
		Name: "sample-kubeconfig",
		Cluster: cpv1alpha2.Cluster{
			Server: fmt.Sprintf("https://%s.example.com:6443", name),
		},
	}}
	meta.SetStatusCondition(&created.Status.Conditions, metav1.Condition{
		Type:    cpv1alpha2.ClusterConditionControlPlaneHealthy,
		Status:  metav1.ConditionTrue,
		Reason:  "AsExpected",
		Message: "control plane is healthy",
	})
	meta.SetStatusCondition(&created.Status.Conditions, metav1.Condition{
		Type:    "Joined",
		Status:  metav1.ConditionTrue,
		Reason:  "ClusterRegistered",
		Message: "cluster is under management",
	})

	if _, err := client.ApisV1alpha2().ClusterProfiles(namespace).UpdateStatus(ctx, created,
		metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("error updating the status of ClusterProfile %q: %w", name, err)
	}

	return nil
}
