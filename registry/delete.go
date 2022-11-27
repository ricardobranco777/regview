package registry

import (
	"context"
	"fmt"
	"net/http"
)

// Delete removes a repository digest from the registry.
// https://docs.docker.com/registry/spec/api/#deleting-an-image
// https://github.com/opencontainers/distribution-spec/blob/main/spec.md#deleting-tags
func (r *Registry) Delete(ctx context.Context, repository string, ref string) (err error) {
	url := r.url("/v2/%s/manifests/%s", repository, ref)
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
