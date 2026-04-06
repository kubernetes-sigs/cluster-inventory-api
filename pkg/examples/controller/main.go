package main

import (
	"flag"
	"log"

	clientcmdv1 "k8s.io/client-go/tools/clientcmd/api/v1"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/pkg/access"
)

func main() {
	providerFile := access.SetupProviderFileFlag()
	flag.Parse()

	accessCfg, err := access.NewFromFile(*providerFile)
	if err != nil {
		log.Fatalf("Got error reading access providers: %v", err)
	}

	// normally we would get this clusterprofile from the local cluster (maybe a watch?)
	// and we would maintain the restconfigs for clusters we're interested in.
	exampleClusterProfile := v1alpha1.ClusterProfile{
		Spec: v1alpha1.ClusterProfileSpec{
			DisplayName: "My Cluster",
		},
		Status: v1alpha1.ClusterProfileStatus{
			AccessProviders: []v1alpha1.AccessProvider{
				{
					Name: "gkeFleet",
					Cluster: clientcmdv1.Cluster{
						Server: "https://myserver.tld:443",
					},
				},
			},
		},
	}

	restConfigForMyCluster, err := accessCfg.BuildConfigFromCP(&exampleClusterProfile)
	if err != nil {
		log.Fatalf("Got error generating restConfig: %v", err)
	}
	log.Printf("Got rest.Config: %v", restConfigForMyCluster)
	// I can then use this rest.Config to build a k8s client.
}
