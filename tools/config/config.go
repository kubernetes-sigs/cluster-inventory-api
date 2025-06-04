package config

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"k8s.io/client-go/util/connrotation"
	"k8s.io/klog/v2"
	"net"
	"net/http"
	"reflect"
	"sigs.k8s.io/cluster-inventory-api/apis/v1alpha1"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins"
	"sigs.k8s.io/cluster-inventory-api/tools/plugins/spiffe"
	"sync"
	"time"
)

const (
	APIServerEndpointPropertyKey = "cluster-entrypoints.k8s.io"
	CABunblePropertyKey          = "cabundle.k8s.io"
	PluginPropertyKey            = "plugin.multicluster.k8s.io"
)

var pluginMap = map[string]plugins.PluginInterface{}

func init() {
	spiffePlugin, err := spiffe.NewPlugin()
	if err == nil {
		pluginMap[spiffePlugin.Name()] = spiffePlugin
	} else {
		utilruntime.HandleError(err)
	}
}

// BuildConfigFromClusterProfile is to build the rest.Config to init the client.
func BuildConfigFromClusterProfile(cluster *v1alpha1.ClusterProfile) (*rest.Config, error) {
	// get the plugin by clusterprofile
	plugin, err := getPluginFromClusterProfile(cluster)
	if err != nil {
		return nil, err
	}

	clusterEndpoint, err := getEndpointFromClusterProfile(cluster)
	if err != nil {
		return nil, err
	}

	config := &rest.Config{
		Host: clusterEndpoint,
		TLSClientConfig: rest.TLSClientConfig{
			CAData: getCABundleFromClusterProfile(cluster),
		},
	}

	a := newAuthenticator(plugin, cluster)
	transportConfig, err := config.TransportConfig()
	if err := a.UpdateTransportConfig(transportConfig); err != nil {
		return nil, err
	}

	return config, nil
}

// code copied from https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/client-go/plugin/pkg/client/auth/exec/exec.go
// to handle token/cert expiration. It caches the token in the authenticator, and check
// expiration upon each call, if it is expired, the plugin is called to refresh.
type roundTripper struct {
	base http.RoundTripper
	a    *Authenticator
}

func (r *roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header.Get("Authorization") != "" {
		return r.base.RoundTrip(req)
	}

	creds, err := r.a.getCreds()
	if err != nil {
		return nil, fmt.Errorf("getting credentials: %v", err)
	}
	if creds.token != "" {
		req.Header.Set("Authorization", "Bearer "+creds.token)
	}

	res, err := r.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized {
		if err := r.a.maybeRefreshCreds(creds); err != nil {
			klog.Errorf("refreshing credentials: %v", err)
		}
	}
	return res, nil
}

type Authenticator struct {
	p       plugins.PluginInterface
	cluster *v1alpha1.ClusterProfile

	cachedCreds *credentials
	mu          sync.Mutex
	exp         time.Time

	now         func() time.Time
	getCert     *transport.GetCertHolder
	dial        *transport.DialHolder
	connTracker *connrotation.ConnectionTracker
}

func newAuthenticator(p plugins.PluginInterface, cluster *v1alpha1.ClusterProfile) *Authenticator {
	connTracker := connrotation.NewConnectionTracker()
	defaultDialer := connrotation.NewDialerWithTracker(
		(&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		connTracker,
	)

	a := &Authenticator{
		cluster: cluster,
		now:     time.Now,
		p:       p,
	}

	a.getCert = &transport.GetCertHolder{GetCert: a.cert}
	a.dial = &transport.DialHolder{Dial: defaultDialer.DialContext}

	return a
}

// UpdateTransportConfig updates the transport.Config to use credentials
// returned by the plugin.
func (a *Authenticator) UpdateTransportConfig(c *transport.Config) error {
	c.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return &roundTripper{
			base: rt,
			a:    a,
		}
	})

	if c.HasCertCallback() {
		return errors.New("can't add TLS certificate callback: transport.Config.TLS.GetCert already set")
	}
	c.TLS.GetCertHolder = a.getCert // comparable for TLS config caching

	if c.DialHolder != nil {
		if c.DialHolder.Dial == nil {
			return errors.New("invalid transport.Config.DialHolder: wrapped Dial function is nil")
		}

		// if c has a custom dialer, we have to wrap it
		// TLS config caching is not supported for this config
		d := connrotation.NewDialerWithTracker(c.DialHolder.Dial, a.connTracker)
		c.DialHolder = &transport.DialHolder{Dial: d.DialContext}
	} else {
		c.DialHolder = a.dial // comparable for TLS config caching
	}

	return nil
}

func (a *Authenticator) getCreds() (*credentials, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cachedCreds != nil && !a.credsExpired() {
		return a.cachedCreds, nil
	}

	if err := a.refreshCredsLocked(); err != nil {
		return nil, err
	}

	return a.cachedCreds, nil
}

func (a *Authenticator) credsExpired() bool {
	if a.exp.IsZero() {
		return false
	}
	return a.now().After(a.exp)
}

// maybeRefreshCreds executes the plugin to force a rotation of the
// credentials, unless they were rotated already.
func (a *Authenticator) maybeRefreshCreds(creds *credentials) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Since we're not making a new pointer to a.cachedCreds in getCreds, no
	// need to do deep comparison.
	if creds != a.cachedCreds {
		// Credentials already rotated.
		return nil
	}

	return a.refreshCredsLocked()
}

func (a *Authenticator) refreshCredsLocked() error {
	cred, err := a.p.Credential(a.cluster)
	if err != nil {
		return err
	}

	if cred.Status == nil {
		return fmt.Errorf("plugin didn't return a status field")
	}
	if cred.Status.Token == "" && cred.Status.ClientCertificateData == "" && cred.Status.ClientKeyData == "" {
		return fmt.Errorf("plugin didn't return a token or cert/key pair")
	}
	if (cred.Status.ClientCertificateData == "") != (cred.Status.ClientKeyData == "") {
		return fmt.Errorf("plugin returned only certificate or key, not both")
	}

	if cred.Status.ExpirationTimestamp != nil {
		a.exp = cred.Status.ExpirationTimestamp.Time
	} else {
		a.exp = time.Time{}
	}

	newCreds := &credentials{
		token: cred.Status.Token,
	}

	if cred.Status.ClientKeyData != "" && cred.Status.ClientCertificateData != "" {
		cert, err := tls.X509KeyPair([]byte(cred.Status.ClientCertificateData), []byte(cred.Status.ClientKeyData))
		if err != nil {
			return fmt.Errorf("failed parsing client key/certificate: %v", err)
		}

		// Leaf is initialized to be nil:
		//  https://golang.org/pkg/crypto/tls/#X509KeyPair
		// Leaf certificate is the first certificate:
		//  https://golang.org/pkg/crypto/tls/#Certificate
		// Populating leaf is useful for quickly accessing the underlying x509
		// certificate values.
		cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("failed parsing client leaf certificate: %v", err)
		}
		newCreds.cert = &cert
	}

	oldCreds := a.cachedCreds
	a.cachedCreds = newCreds
	// Only close all connections when TLS cert rotates. Token rotation doesn't
	// need the extra noise.
	if oldCreds != nil && !reflect.DeepEqual(oldCreds.cert, a.cachedCreds.cert) {
		a.connTracker.CloseAll()
	}

	return nil
}

type credentials struct {
	token string
	cert  *tls.Certificate
}

func (a *Authenticator) cert() (*tls.Certificate, error) {
	creds, err := a.getCreds()
	if err != nil {
		return nil, err
	}
	return creds.cert, nil
}

func getCABundleFromClusterProfile(cluster *v1alpha1.ClusterProfile) []byte {
	caString, found := getPropertyByKey(cluster, CABunblePropertyKey)
	if !found {
		return []byte{}
	}
	return []byte(caString)
}

func getPluginFromClusterProfile(cluster *v1alpha1.ClusterProfile) (plugins.PluginInterface, error) {
	pluginName, found := getPropertyByKey(cluster, PluginPropertyKey)
	if !found {
		return nil, fmt.Errorf("plugin name not found in the clusterproperty %s", cluster.Name)
	}

	plugin, ok := pluginMap[pluginName]
	if !ok {
		return nil, errors.New("plugin not found")
	}
	return plugin, nil
}

// endpoints is a json array string based on https://github.com/kubernetes/enhancements/pull/5185
func getEndpointFromClusterProfile(cluster *v1alpha1.ClusterProfile) (string, error) {
	endpointsStr, found := getPropertyByKey(cluster, APIServerEndpointPropertyKey)
	if !found {
		return "", errors.New("no endpoint property found")
	}

	var endpoints []string
	err := json.Unmarshal([]byte(endpointsStr), &endpoints)
	if err != nil {
		return "", err
	}
	if len(endpoints) == 0 {
		return "", errors.New("no endpoint found")
	}
	// return the first item.
	return endpoints[0], nil
}

func getPropertyByKey(cluster *v1alpha1.ClusterProfile, key string) (string, bool) {
	for _, property := range cluster.Status.Properties {
		if property.Name == key {
			return property.Value, true
		}
	}
	return "", false
}
