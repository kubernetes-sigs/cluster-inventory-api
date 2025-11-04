package main

import (
	"context"
	"flag"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sclient "k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ciaclient "sigs.k8s.io/cluster-inventory-api/client/clientset/versioned"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentials"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func main() {
	// Flags
	credentialsProviders := credentials.SetupProviderFileFlag()
	namespace := flag.String("namespace", "default", "Namespace of the ClusterProfile on the hub cluster")
	clusterProfileName := flag.String("clusterprofile", "", "Name of the ClusterProfile to target (required)")
	flag.Parse()

	if *clusterProfileName == "" {
		log.Fatalf("-clusterprofile is required")
	}

	// Load providers file
	cpCreds, err := credentials.NewFromFile(*credentialsProviders)
	if err != nil {
		log.Fatalf("Got error reading credentials providers: %v", err)
	}

	// Build hub client and get ClusterProfile
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	hubClientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	hubConfig, err := hubClientConfig.ClientConfig()
	if err != nil {
		log.Fatalf("failed to load default kubeconfig for hub: %v", err)
	}
	cic, err := ciaclient.NewForConfig(hubConfig)
	if err != nil {
		log.Fatalf("failed to construct cluster-inventory client: %v", err)
	}
	cp, err := cic.ApisV1alpha1().ClusterProfiles(*namespace).Get(context.Background(), *clusterProfileName, metav1.GetOptions{})
	if err != nil {
		log.Fatalf("failed to get ClusterProfile %s/%s: %v", *namespace, *clusterProfileName, err)
	}

	// Build rest.Config for the spoke cluster using the credential provider
	spokeConfig, err := cpCreds.BuildConfigFromCP(cp)
	if err != nil {
		log.Fatalf("Got error generating spoke rest.Config: %v", err)
	}

	// Example using client-go: Create a Kubernetes client for the spoke cluster and list pods
	mclient, err := k8sclient.NewForConfig(spokeConfig)
	if err != nil {
		log.Fatalf("failed to create spoke client: %v", err)
	}
	plist, err := mclient.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list pods on spoke: %v", err)
	}
	log.Printf("[client-go] Listed %d pods on spoke cluster", len(plist.Items))
	for _, p := range plist.Items {
		log.Printf("[client-go] pod: %s/%s", p.Namespace, p.Name)
	}

	// Example using controller-runtime client
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Fatalf("failed to add core scheme: %v", err)
	}
	crc, err := crclient.New(spokeConfig, crclient.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("failed to create controller-runtime client: %v", err)
	}
	var crPodList corev1.PodList
	if err := crc.List(context.Background(), &crPodList); err != nil {
		log.Fatalf("failed to list pods with controller-runtime: %v", err)
	}
	log.Printf("[controller-runtime] Listed %d pods on spoke cluster", len(crPodList.Items))
	for _, p := range crPodList.Items {
		log.Printf("[controller-runtime] pod: %s/%s", p.Namespace, p.Name)
	}
}
