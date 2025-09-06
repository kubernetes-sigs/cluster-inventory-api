package credentialplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientauthv1beta1 "k8s.io/client-go/pkg/apis/clientauthentication/v1beta1"
)

// Utilities
func errPrintf(plugin string, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "["+plugin+"] "+format+"\n", a...)
}

// readExecInfo reads ExecCredential from KUBERNETES_EXEC_INFO
func readExecInfo() (*clientauthv1beta1.ExecCredential, error) {
	val := os.Getenv("KUBERNETES_EXEC_INFO")
	if strings.TrimSpace(val) == "" {
		return nil, errors.New("KUBERNETES_EXEC_INFO is empty. set provideClusterInfo: true")
	}
	var info clientauthv1beta1.ExecCredential
	if err := json.Unmarshal([]byte(val), &info); err != nil {
		return nil, fmt.Errorf("failed to parse KUBERNETES_EXEC_INFO: %w", err)
	}
	if info.Spec.Cluster == nil || strings.TrimSpace(info.Spec.Cluster.Server) == "" || info.Spec.Cluster.Server == "null" {
		return nil, errors.New("spec.cluster.server is missing in KUBERNETES_EXEC_INFO")
	}
	return &info, nil
}

// Provider defines the common interface for all credential plugins
type Provider interface {
	Name() string
	GetToken(ctx context.Context, in clientauthv1beta1.ExecCredential) (clientauthv1beta1.ExecCredentialStatus, error)
}

// Run is the common entrypoint used by all provider-specific binaries
func Run(p Provider) {
	plugin := strings.TrimSpace(p.Name())
	if plugin == "" {
		fmt.Fprintln(os.Stderr, "[credentialplugin] provider Name() returned empty string; this is not allowed")
		os.Exit(1)
	}

	info, err := readExecInfo()
	if err != nil {
		errPrintf(plugin, "%v", err)
		os.Exit(1)
	}

	status, err := p.GetToken(context.Background(), *info)
	if err != nil {
		errPrintf(plugin, "%v", err)
		os.Exit(1)
	}

	// Build ExecCredential JSON from returned status
	ec := &clientauthv1beta1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clientauthv1beta1.SchemeGroupVersion.Identifier(),
			Kind:       "ExecCredential",
		},
		Status: &status,
	}
	b, err := json.Marshal(ec)
	if err != nil {
		errPrintf(plugin, "failed to marshal ExecCredential: %v", err)
		os.Exit(1)
	}

	w := bufio.NewWriter(os.Stdout)
	_, _ = w.Write(b)
	_ = w.WriteByte('\n')
	_ = w.Flush()
}

// BuildExecCredentialJSON constructs a minimal ExecCredential JSON
func BuildExecCredentialJSON(token string, expiration time.Time) ([]byte, error) {
	ec := &clientauthv1beta1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clientauthv1beta1.SchemeGroupVersion.Identifier(),
			Kind:       "ExecCredential",
		},
		Status: &clientauthv1beta1.ExecCredentialStatus{
			Token: token,
		},
	}
	if !expiration.IsZero() {
		metaExp := metav1.NewTime(expiration.UTC())
		ec.Status.ExpirationTimestamp = &metaExp
	}
	return json.Marshal(ec)
}
