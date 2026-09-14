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
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakeclient "k8s.io/client-go/kubernetes/fake"
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

const kubeconfigTokenUser = `
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
users:
- name: test-user
  user:
    token: s3cr3t-token
`

const kubeconfigNoCurrentContext = `
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: s3cr3t-token
`

const kubeconfigMissingUser = `
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: missing-user
current-context: test-context
users:
- name: test-user
  user:
    token: s3cr3t-token
`

const kubeconfigNoAuthMethod = `
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
users:
- name: test-user
  user: {}
`

const kubeconfigWithExtensions = `
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
users:
- name: test-user
  user:
    token: s3cr3t-token
extensions:
- name: some-extension
  extension:
    foo: bar
`

func kubeconfigWithClientCert(certB64, keyB64 string) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
users:
- name: test-user
  user:
    client-certificate-data: %s
    client-key-data: %s
`, certB64, keyB64)
}

func kubeconfigClientCertFilePath() string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- name: test-cluster
  cluster:
    server: https://example.com:6443
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
current-context: test-context
users:
- name: test-user
  user:
    client-certificate: /var/run/cert.pem
    client-key-data: %s
`, base64.StdEncoding.EncodeToString([]byte("key-bytes")))
}

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

func secretWithKubeconfig(data string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "ns"},
		Data:       map[string][]byte{"kubeconfig": []byte(data)},
	}
}

func rawConfig(name, key string) []byte {
	return []byte(fmt.Sprintf(`{"name":%q,"key":%q}`, name, key))
}

func TestGetToken(t *testing.T) {
	ctx := context.Background()

	t.Run("uninitialized client", func(t *testing.T) {
		p := Provider{}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
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

	t.Run("missing name", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "missing name") {
			t.Fatalf("got %v, want a missing-name error", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "")))
		if err == nil || !strings.Contains(err.Error(), "missing key") {
			t.Fatalf("got %v, want a missing-key error", err)
		}
	})

	t.Run("secret not found", func(t *testing.T) {
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("missing", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "failed to get secret ns/missing") {
			t.Fatalf("got %v, want a secret-not-found error", err)
		}
	})

	t.Run("invalid kubeconfig data", func(t *testing.T) {
		secret := secretWithKubeconfig("not-a-kubeconfig: [")
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "failed to parse kubeconfig") {
			t.Fatalf("got %v, want a parse-failure error", err)
		}
	})

	t.Run("unsupported extensions", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigWithExtensions)
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "kubeconfig extensions are not supported") {
			t.Fatalf("got %v, want an unsupported-extensions error", err)
		}
	})

	t.Run("no context and no current-context", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigNoCurrentContext)
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "no context specified and no current-context") {
			t.Fatalf("got %v, want a no-context error", err)
		}
	})

	t.Run("user not found in kubeconfig", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigMissingUser)
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), `user "missing-user" not found in kubeconfig`) {
			t.Fatalf("got %v, want a user-not-found error", err)
		}
	})

	t.Run("no authentication method found", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigNoAuthMethod)
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "no authentication method found") {
			t.Fatalf("got %v, want a no-auth-method error", err)
		}
	})

	t.Run("client-certificate file path is unsupported", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigClientCertFilePath())
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		_, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err == nil || !strings.Contains(err.Error(), "client-certificate file path is not supported") {
			t.Fatalf("got %v, want a file-path-unsupported error", err)
		}
	})

	t.Run("token auth success", func(t *testing.T) {
		secret := secretWithKubeconfig(kubeconfigTokenUser)
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		status, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.Token != "s3cr3t-token" {
			t.Errorf("Token = %q, want %q", status.Token, "s3cr3t-token")
		}
	})

	t.Run("client certificate auth success", func(t *testing.T) {
		certB64 := base64.StdEncoding.EncodeToString([]byte("cert-bytes"))
		keyB64 := base64.StdEncoding.EncodeToString([]byte("key-bytes"))
		secret := secretWithKubeconfig(kubeconfigWithClientCert(certB64, keyB64))
		p := Provider{KubeClient: fakeclient.NewSimpleClientset(secret), Namespace: "ns"}
		status, err := p.GetToken(ctx, execCredentialWithConfig(rawConfig("s", "kubeconfig")))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if status.ClientCertificateData != "cert-bytes" {
			t.Errorf("ClientCertificateData = %q, want %q", status.ClientCertificateData, "cert-bytes")
		}
		if status.ClientKeyData != "key-bytes" {
			t.Errorf("ClientKeyData = %q, want %q", status.ClientKeyData, "key-bytes")
		}
	})
}

func TestName(t *testing.T) {
	if got := (Provider{}).Name(); got != ProviderName {
		t.Errorf("Name() = %q, want %q", got, ProviderName)
	}
}
