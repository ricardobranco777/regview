package registry

import (
	"context"
	"fmt"
	"net/http"
)

// Delete removes a repository digest from the registry.
// https://docs.docker.com/registry/spec/api/#deleting-an-image
func (r *Registry) Delete(ctx context.Context, repository string, digest string) (err error) {
	url := r.url("/v2/%s/manifests/%s", repository, digest)
	resp, err := r.httpDelete(ctx, url, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusAccepted || resp.StatusCode == http.StatusNotFound {
		return nil
	}

	return fmt.Errorf("got status code: %d", resp.StatusCode)
}
