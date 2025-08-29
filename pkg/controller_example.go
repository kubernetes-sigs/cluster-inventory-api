package main

import (
	"context"
	"encoding/base64"
	"flag"
	"log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentials"
)

func main() {
	credentialsProviders := credentials.SetupProviderFileFlag()
	flag.Parse()

	cpCreds, err := credentials.NewFromFile(*credentialsProviders)
	if err != nil {
		log.Fatalf("Got error reading credentials providers: %v", err)
	}

	caPEMBase64 := `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1J...`
	caPEM, err := base64.StdEncoding.DecodeString(caPEMBase64)
	if err != nil {
		log.Fatalf("CA PEM base64 decode failed: %v", err)
	}

	// normally we would get this clusterprofile from the local cluster (maybe a watch?)
	// and we would maintain the restconfigs for clusters we're interested in.
	exampleClusterProfile := v1alpha1.ClusterProfile{
		Spec: v1alpha1.ClusterProfileSpec{
			DisplayName: "My Cluster",
		},
		Status: v1alpha1.ClusterProfileStatus{
			CredentialProviders: []v1alpha1.CredentialProvider{
				{
					Name: "eks",
					Cluster: clientcmdv1.Cluster{
						Server: "https://xxx.gr7.ap-northeast-1.eks.amazonaws.com",
						CertificateAuthorityData: caPEM,
					},
				},
			},
		},
	}

	restConfigForMyCluster, err := cpCreds.BuildConfigFromCP(&exampleClusterProfile)
	if err != nil {
		log.Fatalf("Got error generating restConfig: %v", err)
	}
	log.Printf("Got credentials: %v", restConfigForMyCluster)
	// I can then use this rest.Config to build a k8s client.

	// Build a client and list Pods in the default namespace
	clientset, err := kubernetes.NewForConfig(restConfigForMyCluster)
	if err != nil {
		log.Fatalf("failed to create clientset: %v", err)
	}
	ctx := context.Background()
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Fatalf("failed to list pods: %v", err)
	}
	log.Printf("default namespace has %d pods", len(pods.Items))
	for i, p := range pods.Items {
		if i >= 10 {
			log.Printf("... (truncated)")
			break
		}
		log.Printf("pod: %s", p.Name)
	}
}
