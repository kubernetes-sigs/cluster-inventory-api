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
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
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

// fakeProvider is a Provider driven entirely by environment variables, so it
// can be reconstructed inside the TestHelperProcess subprocess below.
type fakeProvider struct {
	name  string
	token string
	err   string
}

func (f fakeProvider) Name() string { return f.name }

func (f fakeProvider) GetToken(_ context.Context, _ clientauthenticationv1.ExecCredential) (clientauthenticationv1.ExecCredentialStatus, error) {
	if f.err != "" {
		return clientauthenticationv1.ExecCredentialStatus{}, errors.New(f.err)
	}
	return clientauthenticationv1.ExecCredentialStatus{Token: f.token}, nil
}

// TestHelperProcess is not a real test. It is re-executed as a subprocess by
// TestRun so that Run's os.Exit calls terminate the subprocess instead of the
// test binary itself.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	Run(fakeProvider{
		name:  os.Getenv("HELPER_NAME"),
		token: os.Getenv("HELPER_TOKEN"),
		err:   os.Getenv("HELPER_ERR"),
	})
	// Run only calls os.Exit on failure paths; a normal return means success,
	// so exit cleanly before the surrounding "go test" harness prints its own
	// PASS/summary output to stdout.
	os.Exit(0)
}

func runHelperProcess(t *testing.T, env map[string]string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	if dir := os.Getenv("GOCOVERDIR"); dir != "" {
		cmd.Env = append(cmd.Env, "GOCOVERDIR="+dir)
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err == nil {
		return outBuf.String(), errBuf.String(), 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode()
	}
	t.Fatalf("failed to run helper process: %v", err)
	return "", "", -1
}

func TestRun(t *testing.T) {
	t.Run("empty provider name exits 1", func(t *testing.T) {
		_, stderr, code := runHelperProcess(t, map[string]string{
			"HELPER_NAME": "  ",
		})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "provider Name() returned empty string") {
			t.Errorf("stderr = %q, want it to mention empty provider name", stderr)
		}
	})

	t.Run("missing exec info exits 1", func(t *testing.T) {
		_, stderr, code := runHelperProcess(t, map[string]string{
			"HELPER_NAME": "fake",
		})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "[fake]") || !strings.Contains(stderr, "KUBERNETES_EXEC_INFO") {
			t.Errorf("stderr = %q, want it to mention plugin name and KUBERNETES_EXEC_INFO", stderr)
		}
	})

	t.Run("GetToken error exits 1", func(t *testing.T) {
		_, stderr, code := runHelperProcess(t, map[string]string{
			"HELPER_NAME":          "fake",
			"HELPER_ERR":           "boom",
			"KUBERNETES_EXEC_INFO": execInfoJSON("https://example.com:6443"),
		})
		if code != 1 {
			t.Fatalf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr, "[fake]") || !strings.Contains(stderr, "boom") {
			t.Errorf("stderr = %q, want it to mention plugin name and the GetToken error", stderr)
		}
	})

	t.Run("success prints ExecCredential JSON and exits 0", func(t *testing.T) {
		stdout, stderr, code := runHelperProcess(t, map[string]string{
			"HELPER_NAME":          "fake",
			"HELPER_TOKEN":         "my-token",
			"KUBERNETES_EXEC_INFO": execInfoJSON("https://example.com:6443"),
		})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0; stderr = %q", code, stderr)
		}

		var ec clientauthenticationv1.ExecCredential
		if err := json.Unmarshal([]byte(stdout), &ec); err != nil {
			t.Fatalf("failed to unmarshal stdout %q: %v", stdout, err)
		}
		if ec.Kind != "ExecCredential" {
			t.Errorf("Kind = %q, want %q", ec.Kind, "ExecCredential")
		}
		if ec.Status == nil || ec.Status.Token != "my-token" {
			t.Fatalf("Status.Token = %+v, want token %q", ec.Status, "my-token")
		}
	})
}
