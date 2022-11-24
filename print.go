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
		log.Printf("%s:%s: %v\n", w.repo, w.tag, err)
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
		//fmt.Printf("  %-12s", strings.TrimPrefix(info.ID, "sha256:")[:12])
		v := strings.SplitN(info.ID, ":", 2)
		fmt.Printf("  %-12s", v[1][:12])
	}
	if opts.verbose {
		if opts.raw {
			fmt.Printf("  %-31s", info.Image.Created.String())
		} else {
			fmt.Printf("  %-31s", prettyTime(info.Image.Created))
		}
	}
	if opts.all {
		fmt.Printf("  %-8s  %s", info.Image.OS, info.Image.Architecture)
	}
	fmt.Println()
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
		format := "%-20s\t%q\n"
		if info.Image.Author != "" {
			fmt.Printf(format, "Author", info.Image.Author)
		}
		if info.Image.Architecture != "" {
			fmt.Printf(format, "Architecture", info.Image.Architecture)
		}
		if info.Image.OS != "" {
			fmt.Printf(format, "OS", info.Image.OS)
		}
		if info.Digest != "" {
			fmt.Printf(format, "Digest", info.Digest)
		}
		fmt.Printf(format, "Id", info.ID)
		if opts.raw {
			if info.Image.Created != nil {
				fmt.Printf(format, "Created", info.Image.Created.String())
			}
			fmt.Printf("%-20s\t%d\n", "Size", info.Size)
		} else {
			if info.Image.Created != nil {
				fmt.Printf(format, "Created", prettyTime(info.Image.Created))
			}
			fmt.Printf(format, "Size", prettySize(info.Size))
		}

		if opts.verbose {
			if len(info.Image.Config.Cmd) > 0 {
				fmt.Printf(format, "Cmd", info.Image.Config.Cmd)
			}
			if len(info.Image.Config.Entrypoint) > 0 {
				fmt.Printf(format, "Entrypoint", info.Image.Config.Entrypoint)
			}
			var ports []string
			for port := range info.Image.Config.ExposedPorts {
				ports = append(ports, port)
			}
			if len(ports) > 0 {
				fmt.Printf(format, "ExposedPorts", ports)
			}
			if len(info.Image.Config.Labels) > 0 {
				fmt.Printf(format, "Labels", info.Image.Config.Labels)
			}
			if info.Image.Config.StopSignal != "" {
				fmt.Printf(format, "StopSignal", info.Image.Config.StopSignal)
			}
			if info.Image.Config.User != "" {
				fmt.Printf(format, "User", info.Image.Config.User)
			}
			var volumes []string
			for volume := range info.Image.Config.Volumes {
				volumes = append(volumes, volume)
			}
			if len(volumes) > 0 {
				fmt.Printf(format, "Volumes", volumes)
			}
			if info.Image.Config.WorkingDir != "" {
				fmt.Printf(format, "WorkingDir", info.Image.Config.WorkingDir)
			}
			for i := range info.Image.History {
				fmt.Printf("History[%d]\t\t%q\n", i, info.Image.History[i].CreatedBy)
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
