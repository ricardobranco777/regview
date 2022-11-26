package registry

import (
	"context"
	"io"
	"net/url"

	"github.com/peterhellberg/link"
	"github.com/ricardobranco777/regview/oci"
)

// Catalog returns the repositories in a registry.
func (r *Registry) Catalog(ctx context.Context, u string) ([]string, error) {
	if u == "" {
		u = "/v2/_catalog"
	}
	uri := r.url(u)

	resp, err := r.httpGet(ctx, uri, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if err := apiError(data); err != nil {
		return nil, err
	}

	var response oci.RepositoryList

	if err := response.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	for _, l := range link.ParseHeader(resp.Header) {
		if l.Rel == "next" {
			unescaped, _ := url.QueryUnescape(l.URI)
			repos, err := r.Catalog(ctx, unescaped)
			if err != nil {
				return nil, err
			}
			response.Repositories = append(response.Repositories, repos...)
		}
	}

	return response.Repositories, nil
}
