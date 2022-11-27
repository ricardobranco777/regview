package registry

import (
	"context"
	"errors"
	"net/http"
)

type header struct {
	key   string
	value string
}

func (r *Registry) httpHead(ctx context.Context, url string, headers []*header) (http.Header, error) {
	resp, err := r.httpMethod(ctx, url, headers, http.MethodHead)
	if err != nil {
		return nil, err
	}
	return resp.Header, nil
}

func (r *Registry) httpGet(ctx context.Context, url string, headers []*header) (*http.Response, error) {
	return r.httpMethod(ctx, url, headers, http.MethodGet)
}

func (r *Registry) httpDelete(ctx context.Context, url string, headers []*header) (*http.Response, error) {
	return r.httpMethod(ctx, url, headers, http.MethodDelete)
}

func (r *Registry) httpMethod(ctx context.Context, url string, headers []*header, method string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	for _, h := range headers {
		req.Header.Add(h.key, h.value)
	}

	resp, err := r.Client.Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	dump(resp)

	if resp.StatusCode >= 400 {
		return resp, errors.New(resp.Status)
	}

	return resp, nil
}
