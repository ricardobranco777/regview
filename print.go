package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/ricardobranco777/regview/oci"
	"github.com/ricardobranco777/regview/registry"
	"github.com/ricardobranco777/regview/repoutils"
	"golang.org/x/exp/slices"
)

var maxWorkers = 10

func loadWorker(ctx context.Context, r *registry.Registry, repo string) []*registry.Info {
	tags, err := r.Tags(ctx, repo)
	if err != nil {
		log.Printf("%s: %v\n", repo, err)
		return []*registry.Info{}
	}
	tags = filterRegex(tags, tagRegex)
	sort.Strings(tags)

	tag2Infos := make(map[string][]*registry.Info)
	var wg sync.WaitGroup
	var m sync.Mutex

	wg.Add(len(tags))
	for _, tag := range tags {
		go func(tag string) {
			defer wg.Done()
			infos, err := getInfos(ctx, r, repo, tag)
			if err != nil {
				// Ignore this error that can happen when manifests may be available but not for this platform
				if err.Error() != "MANIFEST_UNKNOWN" {
					log.Printf("%s:%s: %v\n", repo, tag, err)
				}
				return
			}
			m.Lock()
			tag2Infos[tag] = infos
			m.Unlock()
		}(tag)
	}
	wg.Wait()

	var xinfos []*registry.Info
	for _, tag := range tags {
		if infos, ok := tag2Infos[tag]; ok {
			xinfos = append(xinfos, infos...)
		}
	}

	if !opts.all && !opts.verbose {
		return xinfos
	}

	id2Blob := make(map[string]*oci.Image)
	for _, infos := range tag2Infos {
		for _, info := range infos {
			id2Blob[info.ID] = nil
		}
	}

	wg.Add(len(id2Blob))
	for id := range id2Blob {
		go func(id string) {
			defer wg.Done()
			blob, err := r.GetImage(ctx, repo, id)
			if err != nil {
				log.Printf("%s@%s: %v\n", repo, id, err)
				return
			}
			m.Lock()
			id2Blob[id] = blob
			m.Unlock()
		}(id)
	}
	wg.Wait()

	for _, info := range xinfos {
		info.Image = id2Blob[info.ID]
	}

	return xinfos
}

func createRegistryClient(ctx context.Context, domain string) (*registry.Registry, error) {
	auth, err := repoutils.GetAuthConfig(opts.username, opts.password, domain)
	if err != nil {
		return nil, err
	}

	// Prevent non-ssl unless explicitly forced
	if !opts.insecure && strings.HasPrefix(auth.ServerAddress, "http:") {
		return nil, fmt.Errorf("attempted to use insecure protocol! Use --insecure option to force")
	}

	return registry.New(ctx, auth, registry.Opt{
		CAFile:     opts.cacert,
		CertFile:   opts.cert,
		KeyFile:    opts.key,
		Debug:      opts.debug,
		Digests:    opts.digests,
		Domain:     domain,
		Insecure:   opts.insecure,
		NonSSL:     opts.insecure,
		Passphrase: opts.keypass,
	})
}

func getInfos(ctx context.Context, r *registry.Registry, repo string, ref string) (infos []*registry.Info, err error) {
	infos, err = r.GetInfoAll(ctx, repo, ref, opts.arch, opts.os)
	if err != nil {
		return []*registry.Info{}, err
	}
	return infos, nil
}

func deleteImage(ctx context.Context, domain string, image string) {
	r, err := createRegistryClient(ctx, domain)
	if err != nil {
		log.Fatal(err)
	}

	repo, ref, _ := repoutils.GetRepoAndRef(image)
	infos, err := getInfos(ctx, r, repo, ref)
	if err != nil {
		log.Fatalf("%s %s: %v\n", repo, ref, err)
	}

	for _, info := range infos {
		fmt.Printf("Deleting %s@%s\n", repo, info.Digest)
		if !opts.dryRun {
			r.Delete(ctx, repo, info.Digest)
		}
	}

	if opts.all {
		// Also delete multi-arch digest
		if infos[0].DigestAll != infos[0].Digest {
			fmt.Printf("Deleting %s@%s\n", infos[0].Repo, infos[0].DigestAll)
			if !opts.dryRun {
				r.Delete(ctx, infos[0].Repo, infos[0].DigestAll)
			}
		}
		// OCI spec allows for deletions of tags
		fmt.Printf("Deleting %s:%s\n", repo, ref)
		if !opts.dryRun {
			r.Delete(ctx, repo, ref)
		}
	}
}

func printHeader() {
	fmt.Printf("%-*s", repoWidth+20, "REPOSITORY:TAG")
	if opts.digests {
		fmt.Printf("  %-72s", "DIGEST")
	}
	if opts.noTrunc {
		fmt.Printf("  %-72s", "IMAGE ID")
	} else {
		fmt.Printf("  %-12s", "IMAGE ID")
	}
	if opts.verbose {
		fmt.Printf("  %-31s", "CREATED")
	}
	if opts.all {
		fmt.Printf("  %-8s  %s", "OS", "ARCH")
	}
	fmt.Println()
}

func printInfo(info *registry.Info) {
	fmt.Printf("%-*s", repoWidth+20, info.Repo+":"+info.Ref)
	if opts.digests {
		fmt.Printf("  %-72s", info.Digest)
	}
	if opts.noTrunc {
		fmt.Printf("  %-72s", info.ID)
	} else {
		v := strings.SplitN(info.ID, ":", 2)
		fmt.Printf("  %-12s", v[1][:12])
	}
	if opts.verbose {
		if info.Image.Created != nil {
			if opts.raw {
				fmt.Printf("  %-31s", info.Image.Created.String())
			} else {
				fmt.Printf("  %-31s", prettyTime(info.Image.Created))
			}
		} else {
			fmt.Printf("  %-31s", "-")
		}
	}
	if opts.all {
		fmt.Printf("  %-8s  %s", info.Image.OS, info.Image.Architecture)
	}
	fmt.Println()
}

func printIt(format string, name string, it any) {
	var value string
	var b []byte

	v := reflect.ValueOf(it)
	if v.Len() == 0 {
		return
	}

	switch v := it.(type) {
	case string:
		value = v
	case map[string]struct{}:
		var ss []string
		for s := range v {
			ss = append(ss, s)
		}
		b, _ = json.Marshal(ss)
	default:
		b, _ = json.Marshal(v)
	}

	if value == "" {
		value = string(b)
	}
	fmt.Printf(format, name, value)
}

func printImage(ctx context.Context, domain string, image string) {
	r, err := createRegistryClient(ctx, domain)
	if err != nil {
		log.Fatal(err)
	}

	repo, ref, _ := repoutils.GetRepoAndRef(image)
	infos, err := getInfos(ctx, r, repo, ref)
	if err != nil {
		log.Fatalf("%s: %v\n", image, err)
	}

	for _, info := range infos {
		if opts.delete {
			fmt.Printf("Deleting %s@%s\n", repo, info.Digest)
			if !opts.dryRun {
				r.Delete(ctx, repo, info.Digest)
			}
			continue
		}

		format := "%-20s\t%s\n"
		if opts.verbose {
			info.Image, _ = r.GetImage(ctx, repo, info.ID)
		}
		if info.Image != nil {
			// We also have to filter by arch & os because the registry may not return a list
			if len(opts.arch) > 0 && !slices.Contains(opts.arch, info.Image.Architecture) {
				continue
			}
			if len(opts.os) > 0 && !slices.Contains(opts.os, info.Image.OS) {
				continue
			}
			printIt(format, "Author", info.Image.Author)
			printIt(format, "Architecture", info.Image.Architecture)
			printIt(format, "OS", info.Image.OS)
		}
		printIt(format, "Digest", info.Digest)
		printIt(format, "DigestAll", info.DigestAll)
		printIt(format, "Id", info.ID)
		if opts.raw {
			printIt("%-20s\t%s\n", "Size", strconv.FormatInt(info.Size, 10))
		} else {
			printIt(format, "Size", prettySize(info.Size))
		}
		if info.Image != nil {
			if info.Image.Created != nil {
				if opts.raw {
					printIt(format, "Created", info.Image.Created.String())
				} else {
					printIt(format, "Created", prettyTime(info.Image.Created))
				}
			}
			printIt(format, "Cmd", info.Image.Config.Cmd)
			printIt(format, "Entrypoint", info.Image.Config.Entrypoint)
			printIt(format, "ExposedPorts", info.Image.Config.ExposedPorts)
			printIt(format, "Labels", info.Image.Config.Labels)
			printIt(format, "StopSignal", info.Image.Config.StopSignal)
			printIt(format, "User", info.Image.Config.User)
			printIt(format, "Volumes", info.Image.Config.Volumes)
			printIt(format, "WorkingDir", info.Image.Config.WorkingDir)
			for i := range info.Image.History {
				fmt.Printf("History[%d]\t\t%s\n", i, info.Image.History[i].CreatedBy)
			}
		}
		fmt.Println()
	}

	if opts.delete {
		// OCI spec allows for deletions of tags
		fmt.Printf("Deleting %s %s\n", repo, ref)
		if !opts.dryRun {
			r.Delete(ctx, repo, ref)
		}
	}
}

func printAll(ctx context.Context, domain string) {
	r, err := createRegistryClient(ctx, domain)
	if err != nil {
		log.Fatal(err)
	}

	repos, err := r.Catalog(ctx, "")
	if err != nil {
		if _, ok := err.(*json.SyntaxError); ok {
			log.Fatalf("domain %s is not a valid registry", r.Domain)
		} else {
			log.Fatal(err)
		}
	}
	repos = filterRegex(repos, repoRegex)
	sort.Strings(repos)

	repoWidth = getMax(repos)
	printHeader()

	channels := make([]chan []*registry.Info, len(repos))
	for i := range repos {
		channels[i] = make(chan []*registry.Info)
	}

	workers := make(chan struct{}, maxWorkers)

	go func() {
		for i, repo := range repos {
			workers <- struct{}{}
			go func(repo string, channel chan []*registry.Info) {
				channel <- loadWorker(ctx, r, repo)
				close(channel)
			}(repo, channels[i])
		}
	}()

	for i := range repos {
		infos := <-channels[i]
		for _, info := range infos {
			if info.Image != nil {
				// We also have to filter by arch & os because the registry may not return a list
				if len(opts.arch) > 0 && !slices.Contains(opts.arch, info.Image.Architecture) {
					continue
				}
				if len(opts.os) > 0 && !slices.Contains(opts.os, info.Image.OS) {
					continue
				}
			}
			printInfo(info)
		}
		<-workers
	}
}
