package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"

	"github.com/ricardobranco777/regview/registry"
	"github.com/ricardobranco777/regview/repoutils"

	concurrently "github.com/tejzpr/ordered-concurrently/v3"
)

type loadWorker struct {
	reg  *registry.Registry
	repo string
	tag  string
}

func (w *loadWorker) Run(ctx context.Context) any {
	infos, err := getInfos(ctx, w.reg, w.repo, w.tag)
	if err != nil {
		// Ignore this error that can happen when manifests may be available but not for this platform
		if err.Error() != "MANIFEST_UNKNOWN" {
			log.Printf("%s:%s: %v\n", w.repo, w.tag, err)
		}
	}
	return infos
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

	switch v := it.(type) {
	case string:
		if v == "" {
			return
		}
		value = v
	case []string:
		if len(v) == 0 {
			return
		}
		b, _ := json.Marshal(v)
		value = string(b)
	case map[string]struct{}:
		var ss []string
		for s := range v {
			ss = append(ss, s)
		}
		if len(ss) == 0 {
			return
		}
		b, _ := json.Marshal(ss)
		value = string(b)
	default:
		b, _ := json.Marshal(v)
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
		log.Fatalf("%s %s: %v\n", repo, ref, err)
	}
	for _, info := range infos {
		format := "%-20s\t%s\n"
		printIt(format, "Author", info.Image.Author)
		printIt(format, "Architecture", info.Image.Architecture)
		printIt(format, "OS", info.Image.OS)
		printIt(format, "Digest", info.Digest)
		printIt(format, "DigestAll", info.DigestAll)
		printIt(format, "Id", info.ID)
		if opts.raw {
			if info.Image.Created != nil {
				printIt(format, "Created", info.Image.Created.String())
			}
			printIt("%-20s\t%d\n", "Size", info.Size)
		} else {
			if info.Image.Created != nil {
				printIt(format, "Created", prettyTime(info.Image.Created))
			}
			printIt(format, "Size", prettySize(info.Size))
		}

		if opts.verbose {
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
			tags, err := r.Tags(ctx, repo)
			if err != nil {
				log.Printf("Get tags of [%s] error: %s\n", repo, err)
				continue
			}
			tags = filterRegex(tags, tagRegex)
			sort.Strings(tags)
			for _, tag := range tags {
				inputChan <- &loadWorker{reg: r, repo: repo, tag: tag}
			}
		}
		close(inputChan)
	}()

	for out := range output {
		infos := out.Value.([]*registry.Info)
		for _, info := range infos {
			printInfo(info)
		}
	}
}
