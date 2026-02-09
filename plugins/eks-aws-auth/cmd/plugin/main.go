package main

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
	"sigs.k8s.io/cluster-inventory-api/pkg/credentialplugin"
)

func main() {
	credentialplugin.Run(Provider{})
}

type Provider struct{}

func (Provider) Name() string { return "eks" }

// extensions config schema passed via ExecCredential.Spec.Cluster.Config
type execConfig struct {
	Region    string `json:"region"`
	ClusterID string `json:"clusterId"`
}

func (Provider) GetToken(ctx context.Context, info clientauthenticationv1.ExecCredential) (clientauthenticationv1.ExecCredentialStatus, error) {
	// Require extensions payload with region and clusterId
	if info.Spec.Cluster == nil || len(info.Spec.Cluster.Config.Raw) == 0 {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("missing ExecCredential.Spec.Cluster.Config (extensions)")
	}
	var cfg execConfig
	if err := json.Unmarshal(info.Spec.Cluster.Config.Raw, &cfg); err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("invalid extensions config: %w", err)
	}
	if cfg.Region == "" || cfg.ClusterID == "" {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("extensions must include region and clusterId")
	}

	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("failed to initialize token generator: %w", err)
	}
	opts := &token.GetTokenOptions{ClusterID: cfg.ClusterID, Region: cfg.Region}
	t, err := gen.GetWithOptions(ctx, opts)
	if err != nil {
		return clientauthenticationv1.ExecCredentialStatus{}, fmt.Errorf("failed to get EKS token: %w", err)
	}
	exp := metav1.NewTime(t.Expiration.UTC())
	return clientauthenticationv1.ExecCredentialStatus{Token: t.Token, ExpirationTimestamp: &exp}, nil
}
