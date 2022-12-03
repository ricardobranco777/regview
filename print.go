package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ricardobranco777/regview/oci"
	"github.com/ricardobranco777/regview/registry"
	"github.com/ricardobranco777/regview/repoutils"
	"golang.org/x/exp/slices"

	concurrently "github.com/tejzpr/ordered-concurrently/v3"
)

type loadWorker struct {
	reg      *registry.Registry
	repo     string
	tagRegex *regexp.Regexp
}

var maxWorkers = 10

func (w *loadWorker) Run(ctx context.Context) any {
	tags, err := w.reg.Tags(ctx, w.repo)
	if err != nil {
		log.Printf("Get tags of [%s] error: %s\n", w.repo, err)
		return nil
	}
	tags = filterRegex(tags, w.tagRegex)
	sort.Strings(tags)

	var xinfos []*registry.Info
	var m sync.Mutex
	var wg sync.WaitGroup

	wg.Add(len(tags))
	for _, tag := range tags {
		go func(tag string) {
			defer wg.Done()
			infos, err := getInfos(ctx, w.reg, w.repo, tag)
			if err != nil {
				// Ignore this error that can happen when manifests may be available but not for this platform
				if err.Error() != "MANIFEST_UNKNOWN" {
					log.Printf("%s:%s: %v\n", w.repo, tag, err)
				}
				return
			}
			m.Lock()
			xinfos = append(xinfos, infos...)
			m.Unlock()
		}(tag)
	}
	wg.Wait()

	id2Blob := make(map[string]*oci.Image)
	for _, info := range xinfos {
		id2Blob[info.ID] = nil
	}

	wg.Add(len(id2Blob))
	for id := range id2Blob {
		go func(id string) {
			defer wg.Done()
			blob, err := w.reg.GetImage(ctx, w.repo, id)
			if err != nil {
				log.Printf("%s@%s: %v\n", w.repo, id, err)
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
		format := "%-20s\t%s\n"
		info.Image, _ = r.GetImage(ctx, repo, info.ID)
		// We also have to filter by arch & os because the registry may not return a list
		if len(opts.arch) > 0 && !slices.Contains(opts.arch, info.Image.Architecture) {
			continue
		}
		if len(opts.os) > 0 && !slices.Contains(opts.os, info.Image.OS) {
			continue
		}
		if info.Image != nil {
			printIt(format, "Author", info.Image.Author)
			printIt(format, "Architecture", info.Image.Architecture)
			printIt(format, "OS", info.Image.OS)
		}
		printIt(format, "Digest", info.Digest)
		printIt(format, "DigestAll", info.DigestAll)
		printIt(format, "Id", info.ID)
		if opts.raw {
			printIt("%-20s\t%d\n", "Size", info.Size)
		} else {
			printIt(format, "Size", prettySize(info.Size))
		}
		if info.Image == nil {
			continue
		}
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
		fmt.Println()
	}
}

func printAll(ctx context.Context, domain string, repoRegex, tagRegex *regexp.Regexp) {
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

	inputChan := make(chan concurrently.WorkFunction)
	output := concurrently.Process(ctx, inputChan, &concurrently.Options{PoolSize: maxWorkers, OutChannelBuffer: maxWorkers})

	go func() {
		for _, repo := range repos {
			inputChan <- &loadWorker{reg: r, repo: repo, tagRegex: tagRegex}
		}
		close(inputChan)
	}()

	for out := range output {
		infos, ok := out.Value.([]*registry.Info)
		if !ok {
			continue
		}
		for _, info := range infos {
			// We also have to filter by arch & os because the registry may not return a list
			if len(opts.arch) > 0 && !slices.Contains(opts.arch, info.Image.Architecture) {
				continue
			}
			if len(opts.os) > 0 && !slices.Contains(opts.os, info.Image.OS) {
				continue
			}
			printInfo(info)
		}
	}
}
