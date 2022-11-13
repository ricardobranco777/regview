package registry

import (
	"context"
	"io"

	"github.com/opencontainers/go-digest"
)

// DownloadLayer downloads a specific layer by digest for a repository.
func (r *Registry) DownloadLayer(ctx context.Context, repository string, digest digest.Digest) (io.ReadCloser, error) {
	url := r.url("/v2/%s/blobs/%s", repository, digest)

	resp, err := r.httpGet(ctx, url, nil)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
