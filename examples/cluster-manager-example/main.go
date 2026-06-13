package main

import (
	"context"
	"flag"
	"log"
	"path/filepath"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// 1. Build rest.Config from kubeconfig
	hubConfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		log.Fatalf("failed to build kubeconfig: %v", err)
	}

	// 2. Initialize the cluster-inventory-api typed client
	cic, err := ciaclient.NewForConfig(hubConfig)
	if err != nil {
		log.Fatalf("failed to construct cluster-inventory client: %v", err)
	}

	// 3. Define the ClusterProfile
	cp := &v1alpha1.ClusterProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-us-west-1",
			Namespace: "cluster-inventory", // Make sure this namespace exists
			Labels: map[string]string{
				v1alpha1.LabelClusterManagerKey: "my-fleet-manager",
			},
		},
		Spec: v1alpha1.ClusterProfileSpec{
			DisplayName: "US West 1 Production Cluster",
			ClusterManager: v1alpha1.ClusterManager{
				Name: "my-fleet-manager",
			},
		},
	}

	// 4. Create the ClusterProfile in the cluster
	ctx := context.Background()
	log.Printf("Creating ClusterProfile %s...", cp.Name)
	createdCP, err := cic.ApisV1alpha1().ClusterProfiles(cp.Namespace).Create(ctx, cp, metav1.CreateOptions{})
	if err != nil {
		log.Fatalf("failed to create ClusterProfile: %v", err)
	}

	log.Printf("Successfully created ClusterProfile: %s\n", createdCP.Name)

	// 5. Update the status subresource
	//    In a real controller, you would fetch this data from the actual spoke cluster.
	//    Here we set example values to demonstrate how the status fields work.

	// 5a. Set the ControlPlaneHealthy condition
	meta.SetStatusCondition(&createdCP.Status.Conditions, metav1.Condition{
		Type:               v1alpha1.ClusterConditionControlPlaneHealthy,
		Status:             metav1.ConditionTrue,
		Reason:             "ClusterReachable",
		Message:            "API server is reachable and responding",
		LastTransitionTime: metav1.Now(),
	})

	// 5b. Set the Kubernetes version
	createdCP.Status.Version = v1alpha1.ClusterVersion{
		Kubernetes: "1.32.0",
	}

	// 5c. Set cluster properties (KEP-2149)
	createdCP.Status.Properties = []v1alpha1.Property{
		{
			Name:             "topology.kubernetes.io/region",
			Value:            "us-west-1",
			LastObservedTime: metav1.Now(),
		},
		{
			Name:             "topology.kubernetes.io/zone",
			Value:            "us-west-1a",
			LastObservedTime: metav1.Now(),
		},
	}

	// 5d. Persist the status update via the status subresource
	log.Println("Updating ClusterProfile status...")
	updatedCP, err := cic.ApisV1alpha1().ClusterProfiles(createdCP.Namespace).UpdateStatus(
		ctx, createdCP, metav1.UpdateOptions{},
	)
	if err != nil {
		log.Fatalf("failed to update ClusterProfile status: %v", err)
	}

	log.Printf("Status updated for ClusterProfile: %s\n", updatedCP.Name)
	log.Printf("  Conditions: %d set", len(updatedCP.Status.Conditions))
	log.Printf("  Version: %s", updatedCP.Status.Version.Kubernetes)
	log.Printf("  Properties: %d set", len(updatedCP.Status.Properties))
}
