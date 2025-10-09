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
	clientauthenticationv1 "k8s.io/client-go/pkg/apis/clientauthentication/v1"
	authexec "k8s.io/client-go/tools/auth/exec"
)

// Utilities
func errPrintf(plugin string, format string, a ...any) {
	fmt.Fprintf(os.Stderr, "["+plugin+"] "+format+"\n", a...)
}

// readExecInfo reads ExecCredential from KUBERNETES_EXEC_INFO
func readExecInfo() (*clientauthenticationv1.ExecCredential, error) {
	obj, _, err := authexec.LoadExecCredentialFromEnv()
	if err != nil {
		return nil, fmt.Errorf("failed to read KUBERNETES_EXEC_INFO: %w", err)
	}
	ec, ok := obj.(*clientauthenticationv1.ExecCredential)
	if !ok {
		return nil, fmt.Errorf("unexpected ExecCredential type (expect v1), got %T", obj)
	}
	if ec.Spec.Cluster == nil || strings.TrimSpace(ec.Spec.Cluster.Server) == "" || ec.Spec.Cluster.Server == "null" {
		return nil, errors.New("spec.cluster.server is missing in KUBERNETES_EXEC_INFO")
	}
	return ec, nil
}

// Provider defines the common interface for all credential plugins
type Provider interface {
	Name() string
	GetToken(ctx context.Context, in clientauthenticationv1.ExecCredential) (clientauthenticationv1.ExecCredentialStatus, error)
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
	ec := &clientauthenticationv1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clientauthenticationv1.SchemeGroupVersion.Identifier(),
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
	ec := &clientauthenticationv1.ExecCredential{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clientauthenticationv1.SchemeGroupVersion.Identifier(),
			Kind:       "ExecCredential",
		},
		Status: &clientauthenticationv1.ExecCredentialStatus{
			Token: token,
		},
	}
	if !expiration.IsZero() {
		metaExp := metav1.NewTime(expiration.UTC())
		ec.Status.ExpirationTimestamp = &metaExp
	}
	return json.Marshal(ec)
}
