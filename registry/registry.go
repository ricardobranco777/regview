package registry

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	types "github.com/docker/docker/api/types/registry"
	"github.com/docker/go-connections/tlsconfig"
)

// Registry defines the client for retrieving information from the registry API.
type Registry struct {
	URL        string
	Domain     string
	Username   string
	Password   string
	Passphrase string
	Client     *http.Client
	Opt        Opt
	useHead    bool // We set it to false if the registry doesn't return digests with HEAD
}

var reProtocol = regexp.MustCompile("^https?://")
var debug = false

// Opt holds the options for a new registry.
type Opt struct {
	Domain     string
	CAFile     string
	CertFile   string
	KeyFile    string
	Passphrase string
	Insecure   bool
	Debug      bool
	Digests    bool
	NonSSL     bool
	Timeout    time.Duration
	Headers    map[string]string
}

// New creates a new Registry struct with the given URL and credentials.
func New(ctx context.Context, auth types.AuthConfig, opt Opt) (*Registry, error) {
	if opt.Debug {
		debug = true
	}

	tlsClientConfig, _ := tlsconfig.Client(
		tlsconfig.Options{
			CAFile:             opt.CAFile,
			CertFile:           opt.CertFile,
			KeyFile:            opt.KeyFile,
			Passphrase:         opt.Passphrase,
			InsecureSkipVerify: opt.Insecure,
		})
	transport := &http.Transport{
		TLSClientConfig: tlsClientConfig,
	}

	return newFromTransport(ctx, auth, transport, opt)
}

func newFromTransport(ctx context.Context, auth types.AuthConfig, transport http.RoundTripper, opt Opt) (*Registry, error) {
	if len(opt.Domain) < 1 || opt.Domain == "docker.io" {
		opt.Domain = auth.ServerAddress
	}
	url := strings.TrimSuffix(opt.Domain, "/")
	authURL := strings.TrimSuffix(auth.ServerAddress, "/")

	if !reProtocol.MatchString(url) {
		if !opt.NonSSL {
			url = "https://" + url
		} else {
			url = "http://" + url
		}
	}

	if !reProtocol.MatchString(authURL) {
		if !opt.NonSSL {
			authURL = "https://" + authURL
		} else {
			authURL = "http://" + authURL
		}
	}

	tokenTransport := &TokenTransport{
		Transport: transport,
		Username:  auth.Username,
		Password:  auth.Password,
	}
	basicAuthTransport := &BasicTransport{
		Transport: tokenTransport,
		URL:       authURL,
		Username:  auth.Username,
		Password:  auth.Password,
	}
	errorTransport := &ErrorTransport{
		Transport: basicAuthTransport,
	}
	customTransport := &CustomTransport{
		Transport: errorTransport,
		Headers:   opt.Headers,
	}

	registry := &Registry{
		URL:    url,
		Domain: reProtocol.ReplaceAllString(url, ""),
		Client: &http.Client{
			Timeout:   opt.Timeout,
			Transport: customTransport,
		},
		Username: auth.Username,
		Password: auth.Password,
		Opt:      opt,
		useHead:  true,
	}

	return registry, nil
}

// url returns a registry URL with the passed arguements concatenated.
func (r *Registry) url(pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	url := fmt.Sprintf("%s%s", r.URL, pathSuffix)
	return url
}
