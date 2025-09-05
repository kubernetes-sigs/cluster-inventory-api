package main

import (
	"sigs.k8s.io/cluster-inventory-api/pkg/credentialplugin"
	provider "sigs.k8s.io/cluster-inventory-api/pkg/secretreader"
)

func main() {
	p, err := provider.NewDefault()
	if err != nil {
		panic(err)
	}
	credentialplugin.Run(*p)
}
