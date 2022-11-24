package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sync"

	"github.com/ricardobranco777/regview/repoutils"
)

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
}

func deleteAll(ctx context.Context, domain string, repoRegex, tagRegex *regexp.Regexp) {
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

	var wg sync.WaitGroup
	wg.Add(len(repos))
	go func() {
		for _, repo := range repos {
			go func(repo string) {
				defer wg.Done()
				tags, err := r.Tags(ctx, repo)
				if err != nil {
					log.Printf("Get tags of [%s] error: %s\n", repo, err)
					return
				}
				tags = filterRegex(tags, tagRegex)
				var wg2 sync.WaitGroup
				wg2.Add(len(tags))
				for _, tag := range tags {
					go func(repo string, tag string) {
						defer wg2.Done()
						infos, err := getInfos(ctx, r, repo, tag)
						if err != nil {
							log.Printf("%s:%s: %v\n", repo, tag, err)
							return
						}
						for _, info := range infos {
							fmt.Printf("Deleting %s@%s\n", repo, info.Digest)
							if !opts.dryRun {
								r.Delete(ctx, repo, info.Digest)
							}
						}
					}(repo, tag)
				}
				wg2.Wait()
			}(repo)
		}
	}()
	wg.Wait()
}
