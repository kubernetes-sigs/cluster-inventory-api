/*
Copyright 2026 The Kubernetes Authors.

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

package main

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

func execCredentialWithConfig(raw []byte) clientauthenticationv1.ExecCredential {
	var cluster *clientauthenticationv1.Cluster
	if raw != nil {
		cluster = &clientauthenticationv1.Cluster{
			Config: runtime.RawExtension{Raw: raw},
		}
	}
	return clientauthenticationv1.ExecCredential{
		Spec: clientauthenticationv1.ExecCredentialSpec{
			Cluster: cluster,
		},
	}
}

func TestGetToken(t *testing.T) {
	ctx := context.Background()

	t.Run("uninitialized client", func(t *testing.T) {
		p := Provider{}
		_, err := p.GetToken(ctx, execCredentialWithConfig([]byte(`{"clusterName":"foo"}`)))
		if err == nil || !strings.Contains(err.Error(), "provider clients are not initialized") {
			t.Fatalf("got %v, want an uninitialized-client error", err)
		}
	})

	t.Run("missing cluster config", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(nil))
		if err == nil || !strings.Contains(err.Error(), "missing ExecCredential.Spec.Cluster.Config") {
			t.Fatalf("got %v, want a missing-config error", err)
		}
	})

	t.Run("invalid config JSON", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig([]byte("not-json")))
		if err == nil || !strings.Contains(err.Error(), "invalid ExecCredential.Spec.Cluster.Config") {
			t.Fatalf("got %v, want an invalid-config error", err)
		}
	})

	t.Run("empty clusterName", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig([]byte(`{"clusterName":""}`)))
		if err == nil || !strings.Contains(err.Error(), "missing clusterName") {
			t.Fatalf("got %v, want a missing-clusterName error", err)
		}
	})

	t.Run("secret not found", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig([]byte(`{"clusterName":"missing"}`)))
		if err == nil || !strings.Contains(err.Error(), "failed to get secret ns/missing") {
			t.Fatalf("got %v, want a secret-not-found error", err)
		}
	})

	t.Run("secret missing token key", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "ns"},
			Data:       map[string][]byte{"other": []byte("value")},
		}
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig([]byte(`{"clusterName":"cluster-a"}`)))
		if err == nil || !strings.Contains(err.Error(), `missing "token" key`) {
			t.Fatalf("got %v, want a missing-token-key error", err)
		}
	})

	t.Run("success", func(t *testing.T) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a", Namespace: "ns"},
			Data:       map[string][]byte{SecretTokenKey: []byte("s3cr3t")},
		}
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		status, err := p.GetToken(ctx, execCredentialWithConfig([]byte(`{"clusterName":"cluster-a"}`)))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Token != "s3cr3t" {
			t.Errorf("Token = %q, want %q", status.Token, "s3cr3t")
		}
	})
}

func TestName(t *testing.T) {
	if got := (Provider{}).Name(); got != ProviderName {
		t.Errorf("Name() = %q, want %q", got, ProviderName)
	}
}
