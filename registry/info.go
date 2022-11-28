package registry

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/distribution/distribution/manifest/manifestlist"
	"github.com/distribution/distribution/manifest/schema2"
	"github.com/hashicorp/golang-lru/v2"
	"github.com/ricardobranco777/regview/oci"
	"golang.org/x/exp/slices"

	digest "github.com/opencontainers/go-digest"
)

// Info type for interesting information
type Info struct {
	Image     *oci.Image
	Digest    string
	DigestAll string
	ID        string
	Repo      string
	Ref       string
	Size      int64
}

// LRU cache for blobs that can be shared by many tags
var cache *lru.Cache[digest.Digest, *oci.Image]

func init() {
	var err error
	cache, err = lru.New[digest.Digest, *oci.Image](128)
	if err != nil {
		panic(err)
	}
}

// Get blob
func (r *Registry) getBlob(ctx context.Context, repo string, ref digest.Digest) (*oci.Image, error) {
	if image, ok := cache.Get(ref); ok {
		return image, nil
	}

	url := r.url("/v2/%s/blobs/%s", repo, ref)
	resp, err := r.httpGet(ctx, url, nil)
	if resp == nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if err := apiError(data, err); err != nil {
		return nil, err
	}

	var image oci.Image
	if err := image.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	cache.Add(ref, &image)
	return &image, nil
}

// Get digest if not available
func (r *Registry) getDigest(ctx context.Context, repo string, ref string, data []byte) digest.Digest {
	// Some registries like Amazon return digests only with HEAD
	if r.useHead {
		url := r.url("/v2/%s/manifests/%s", repo, ref)
		headers := []*header{{"Accept", schema2.MediaTypeManifest}}
		h, err := r.httpHead(ctx, url, headers)
		if err == nil {
			d, _ := digest.Parse(h.Get("Docker-Content-Digest"))
			if d != "" {
				return d
			}
			r.useHead = false
		}
	}

	// Some stupid registries like RedHat's don't return a digest at all
	return digest.FromBytes(data)
}

// Get Info from manifest
func (r *Registry) getInfo(ctx context.Context, m *oci.Manifest, header http.Header, repo string, ref string, more bool) (*Info, error) {
	if m.Versioned.SchemaVersion != 2 {
		err := errors.New("invalid schema version")
		return nil, err
	}

	info := &Info{
		Repo: repo,
		Ref:  ref,
		ID:   m.Config.Digest.String(),
	}

	for _, layer := range m.Layers {
		info.Size += layer.Size
	}

	if strings.Contains(ref, ":") {
		info.Digest = ref
	} else {
		d, _ := digest.Parse(header.Get("Docker-Content-Digest"))
		info.Digest = d.String()
	}

	if !more {
		return info, nil
	}

	var err error
	info.Image, err = r.getBlob(ctx, repo, m.Config.Digest)

	return info, err
}

// GetInfo from manifest
func (r *Registry) GetInfo(ctx context.Context, repo string, ref string, more bool) (*Info, error) {
	url := r.url("/v2/%s/manifests/%s", repo, ref)
	headers := []*header{
		{"Accept", schema2.MediaTypeManifest},
		{"Accept", oci.MediaTypeImageManifest},
	}
	resp, err := r.httpGet(ctx, url, headers)
	if resp == nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if err := apiError(data, err); err != nil {
		return nil, err
	}

	var m oci.Manifest
	if err := m.UnmarshalJSON(data); err != nil {
		return nil, err
	}

	info, err := r.getInfo(ctx, &m, resp.Header, repo, ref, more)
	if err != nil {
		return nil, err
	}

	if info.Digest == "" && r.Opt.Digests {
		info.Digest = r.getDigest(ctx, repo, ref, data).String()
	}

	return info, nil
}

// GetInfoAll from fat manifests and then each manifest
func (r *Registry) GetInfoAll(ctx context.Context, repo string, ref string, more bool, arches []string, oses []string) ([]*Info, error) {
	url := r.url("/v2/%s/manifests/%s", repo, ref)
	headers := []*header{
		{"Accept", manifestlist.MediaTypeManifestList},
		{"Accept", oci.MediaTypeImageIndex},
		{"Accept", schema2.MediaTypeManifest},
		{"Accept", oci.MediaTypeImageManifest},
	}
	resp, err := r.httpGet(ctx, url, headers)
	if resp == nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if err := apiError(data, err); err != nil {
		return nil, err
	}

	var m oci.Index
	if resp.Header.Get("Content-Type") == manifestlist.MediaTypeManifestList {
		if err := m.UnmarshalJSON(data); err != nil {
			return nil, err
		}
	} else if resp.Header.Get("Content-Type") == schema2.MediaTypeManifest {
		var m oci.Manifest
		if err := m.UnmarshalJSON(data); err != nil {
			return nil, err
		}

		info, err := r.getInfo(ctx, &m, resp.Header, repo, ref, more)
		if err != nil {
			return nil, err
		}
		return []*Info{info}, nil
	}

	d, err := digest.Parse(resp.Header.Get("Docker-Content-Digest"))
	if err != nil && r.Opt.Digests {
		d = r.getDigest(ctx, repo, ref, data)
	}

	var wg sync.WaitGroup

	infos := make([]*Info, len(m.Manifests))
	for i, manifest := range m.Manifests {
		if len(arches) > 0 && !slices.Contains(arches, manifest.Platform.Architecture) || len(oses) > 0 && !slices.Contains(oses, manifest.Platform.OS) {
			continue
		}
		// Avoid address being captured in for loop
		manifest := manifest
		wg.Add(1)
		go func(i int, manifest *oci.Descriptor) {
			defer wg.Done()
			info, err := r.GetInfo(ctx, repo, manifest.Digest.String(), more)
			if err != nil {
				log.Printf("ERROR: %s@%s: %v", repo, manifest.Digest.String(), err)
				return
			}
			info.DigestAll = d.String()
			info.Ref = ref
			infos[i] = info
		}(i, &manifest)
	}
	wg.Wait()

	xinfos := make([]*Info, 0, len(infos))
	for i := range infos {
		if infos[i] != nil {
			xinfos = append(xinfos, infos[i])
		}
	}
	return xinfos, nil
}
