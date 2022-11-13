package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/distribution/distribution/manifest/manifestlist"
	"github.com/distribution/distribution/manifest/schema2"
	"github.com/docker/docker/api/types"
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
	}

	return registry, nil
}

// url returns a registry URL with the passed arguements concatenated.
func (r *Registry) url(pathTemplate string, args ...interface{}) string {
	pathSuffix := fmt.Sprintf(pathTemplate, args...)
	url := fmt.Sprintf("%s%s", r.URL, pathSuffix)
	return url
}

func (r *Registry) getJSON(ctx context.Context, url string, response interface{}) (http.Header, error) {
	var mediaType string
	switch response.(type) {
	case *schema2.Manifest:
		mediaType = schema2.MediaTypeManifest
	case *manifestlist.ManifestList:
		mediaType = manifestlist.MediaTypeManifestList
	}

	headers := []*header{{"Accept", mediaType}}

	resp, err := r.httpGet(ctx, url, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return nil, err
	}

	return resp.Header, nil
}
