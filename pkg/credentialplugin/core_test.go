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

package credentialplugin

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
)

func execInfoJSON(server string) string {
	return `{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential",` +
		`"spec":{"cluster":{"server":"` + server + `"}}}`
}

func TestReadExecInfo(t *testing.T) {
	tests := []struct {
		name    string
		envVal  string
		unset   bool
		wantErr string
	}{
		{
			name:    "env var unset",
			unset:   true,
			wantErr: "failed to read KUBERNETES_EXEC_INFO",
		},
		{
			name:    "malformed JSON",
			envVal:  "not-json",
			wantErr: "failed to read KUBERNETES_EXEC_INFO",
		},
		{
			name:    "empty server",
			envVal:  execInfoJSON(""),
			wantErr: "spec.cluster.server is missing",
		},
		{
			name:    "server is literal null",
			envVal:  execInfoJSON("null"),
			wantErr: "spec.cluster.server is missing",
		},
		{
			name:   "valid server",
			envVal: execInfoJSON("https://example.com:6443"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				t.Setenv("KUBERNETES_EXEC_INFO", "")
			} else {
				t.Setenv("KUBERNETES_EXEC_INFO", tt.envVal)
			}

			ec, err := readExecInfo()

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ec == nil || ec.Spec.Cluster == nil {
				t.Fatal("expected a populated ExecCredential with a cluster")
			}
			if ec.Spec.Cluster.Server != "https://example.com:6443" {
				t.Errorf("Server = %q, want %q", ec.Spec.Cluster.Server, "https://example.com:6443")
			}
		})
	}
}

func TestBuildExecCredentialJSON(t *testing.T) {
	t.Run("token without expiration", func(t *testing.T) {
		b, err := BuildExecCredentialJSON("my-token", time.Time{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var ec clientauthenticationv1.ExecCredential
		if err := json.Unmarshal(b, &ec); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}
		if ec.Kind != "ExecCredential" {
			t.Errorf("Kind = %q, want %q", ec.Kind, "ExecCredential")
		}
		if ec.APIVersion != clientauthenticationv1.SchemeGroupVersion.Identifier() {
			t.Errorf("APIVersion = %q, want %q", ec.APIVersion, clientauthenticationv1.SchemeGroupVersion.Identifier())
		}
		if ec.Status == nil || ec.Status.Token != "my-token" {
			t.Fatalf("Status.Token = %+v, want token %q", ec.Status, "my-token")
		}
		if ec.Status.ExpirationTimestamp != nil {
			t.Errorf("ExpirationTimestamp = %v, want nil", ec.Status.ExpirationTimestamp)
		}
	})

	t.Run("token with expiration", func(t *testing.T) {
		exp := time.Date(2030, time.January, 2, 3, 4, 5, 0, time.UTC)
		b, err := BuildExecCredentialJSON("my-token", exp)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var ec clientauthenticationv1.ExecCredential
		if err := json.Unmarshal(b, &ec); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}
		if ec.Status == nil || ec.Status.ExpirationTimestamp == nil {
			t.Fatal("expected a non-nil ExpirationTimestamp")
		}
		if !ec.Status.ExpirationTimestamp.Time.Equal(exp) {
			t.Errorf("ExpirationTimestamp = %v, want %v", ec.Status.ExpirationTimestamp.Time, exp)
		}
	})
}
