package registry

import (
	"context"
	"io"
	"net/url"

	"github.com/peterhellberg/link"
	"github.com/ricardobranco777/regview/oci"
)

func (r *Registry) tags(ctx context.Context, u string, repository string) ([]string, error) {
	var uri string
	if u == "" {
		uri = r.url("/v2/%s/tags/list", repository)
	} else {
		uri = r.url(u)
	}

	resp, err := r.httpGet(ctx, uri, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	dump(resp)

	data, _ := io.ReadAll(resp.Body)
	if err := apiError(data, err); err != nil {
		return nil, err
	}

	var response oci.TagList

	if err := response.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	for _, l := range link.ParseHeader(resp.Header) {
		if l.Rel == "next" {
			unescaped, _ := url.QueryUnescape(l.URI)
			tags, err := r.tags(ctx, unescaped, repository)
			if err != nil {
				return nil, err
			}
			response.Tags = append(response.Tags, tags...)
		}
	}

	return response.Tags, nil
}

// Tags returns the tags for a specific repository.
func (r *Registry) Tags(ctx context.Context, repository string) ([]string, error) {
	return r.tags(ctx, "", repository)
}
