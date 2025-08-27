package secrets

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
)

const (
	// Label key that must be set on the namespace hosting the secret
	LabelConsumerKey = "x-k8s.io/cluster-inventory-consumer"

	// Label key that must be set on the secret to point to the ClusterProfile name
	LabelClusterProfileName = "x-k8s.io/cluster-profile"

	// Optional label key on the secret to indicate the ClusterProfile namespace
	// If absent, the ClusterProfile is assumed to be in the "default" namespace
	LabelClusterProfileNamespace = "x-k8s.io/cluster-profile-namespace"

	// Data key within the secret that stores kubeconfig content
	// Note: The KEP refers to this field as "Config" semantically,
	// however, the conventional secret data key is lowercase "config"
	// and is what client-go helpers expect in examples and tests.
	SecretDataKeyKubeconfig = "config"
)

// BuildConfigFromCP discovers a Secret that contains kubeconfig based on:
// - Namespace label: the hosting namespace MUST have label LabelConsumerKey=<consumerName>
// - Secret labels:
//   - MUST have label LabelClusterProfileName=<clusterProfile.Name>
//   - MAY have label LabelClusterProfileNamespace=<clusterProfile.Namespace>
//     If absent, the ClusterProfile is assumed to be in namespace "default".
// The kubeconfig must be stored under the Secret data key "config".
// Returns a *rest.Config if exactly one matching secret is found; otherwise returns an error.
// Spec reference (KEP-4322 Secret format):
// https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/4322-cluster-inventory/README.md#secret-format
func BuildConfigFromCP(
	ctx context.Context,
	kubeClient kubernetes.Interface,
	consumerName string,
	clusterProfile *v1alpha1.ClusterProfile,
) (*rest.Config, error) {
	if kubeClient == nil {
		return nil, fmt.Errorf("kubeClient must not be nil")
	}
	if clusterProfile == nil {
		return nil, fmt.Errorf("clusterProfile must not be nil")
	}
	if consumerName == "" {
		return nil, fmt.Errorf("consumerName must be provided")
	}

	cpName := clusterProfile.Name
	if cpName == "" {
		return nil, fmt.Errorf("clusterProfile.Name must be provided")
	}
	cpNamespace := clusterProfile.Namespace
	if cpNamespace == "" {
		cpNamespace = corev1.NamespaceDefault
	}

	// 1) Find all namespaces for this consumer
	nsSelector := labels.Set{LabelConsumerKey: consumerName}.AsSelector().String()
	nsList, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{LabelSelector: nsSelector})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces for consumer %q: %w", consumerName, err)
	}
	if len(nsList.Items) == 0 {
		return nil, fmt.Errorf("no namespaces found labeled %q=%q", LabelConsumerKey, consumerName)
	}

	// 2) For each namespace, find secrets labeled for this ClusterProfile
	var candidates []corev1.Secret
	secSelector := labels.Set{LabelClusterProfileName: cpName}.AsSelector().String()
	for _, ns := range nsList.Items {
		sList, err := kubeClient.CoreV1().Secrets(ns.Name).List(ctx, metav1.ListOptions{LabelSelector: secSelector})
		if err != nil {
			return nil, fmt.Errorf("failed to list secrets in namespace %s: %w", ns.Name, err)
		}
		for _, s := range sList.Items {
			// If the secret specifies a CP namespace label, it must match cpNamespace.
			// If it does not specify, it is considered to target the "default" namespace only.
			labeledNs, hasNsLabel := s.Labels[LabelClusterProfileNamespace]
			if hasNsLabel {
				if labeledNs == cpNamespace {
					candidates = append(candidates, s)
				}
				continue
			}
			// No namespace label present. Only accept if CP is in default namespace.
			if cpNamespace == corev1.NamespaceDefault {
				candidates = append(candidates, s)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no secret found for ClusterProfile %s/%s for consumer %q", cpNamespace, cpName, consumerName)
	}
	if len(candidates) > 1 {
		return nil, fmt.Errorf("multiple secrets found for ClusterProfile %s/%s for consumer %q; expected exactly one", cpNamespace, cpName, consumerName)
	}

	// 3) Build rest.Config from the single matching secret
	selected := candidates[0]
	raw, ok := selected.Data[SecretDataKeyKubeconfig]
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("secret %s/%s does not contain kubeconfig under key %q", selected.Namespace, selected.Name, SecretDataKeyKubeconfig)
	}
	restCfg, err := clientcmd.RESTConfigFromKubeConfig(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to build rest.Config from kubeconfig in secret %s/%s: %w", selected.Namespace, selected.Name, err)
	}
	return restCfg, nil
}

