package main

import (
	"context"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"syscall"

	"github.com/ricardobranco777/regview/registry"
	"github.com/ricardobranco777/regview/repoutils"
	"golang.org/x/exp/slices"
	"mvdan.cc/sh/v3/pattern"
)

import flag "github.com/spf13/pflag"

const version = "2.9"

var opts struct {
	all      bool
	delete   bool
	debug    bool
	digests  bool
	dryRun   bool
	insecure bool
	noTrunc  bool
	raw      bool
	verbose  bool
	version  bool
	cacert   string
	cert     string
	key      string
	username string
	password string
	keypass  string
	arch     []string
	os       []string
}

var repoWidth int

// ContextKey type for contexts
type ContextKey string

func init() {
	log.SetFlags(0)

	arches := []string{"386", "amd64", "arm", "arm64", "mips", "mips64", "mips64le", "mipsle", "ppc64", "ppc64le", "riscv64", "s390x", "wasm"}
	oses := []string{"aix", "android", "darwin", "dragonfly", "freebsd", "illumos", "ios", "js", "linux", "netbsd", "openbsd", "plan9", "solaris", "windows"}

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: [OPTIONS] %s REGISTRY[/REPOSITORY[:TAG|@DIGEST]]\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "Valid options for --arch: %s\n", strings.Join(arches, " "))
		fmt.Fprintf(os.Stderr, "Valid options for --os: %s\n", strings.Join(oses, " "))
	}
	flag.BoolVarP(&opts.all, "all", "a", false, "Print information for all architecture")
	flag.BoolVarP(&opts.delete, "delete", "", false, "Delete images. USE WITH CAUTION")
	flag.BoolVarP(&opts.debug, "debug", "", false, "Enable debug")
	flag.BoolVarP(&opts.digests, "digests", "", false, "Show digests")
	flag.BoolVarP(&opts.dryRun, "dry-run", "", false, "Used with --delete: only show the images that would be deleted")
	flag.BoolVarP(&opts.insecure, "insecure", "", false, "Allow insecure server connections")
	flag.BoolVarP(&opts.noTrunc, "no-trunc", "", false, "Don't truncate output")
	flag.BoolVarP(&opts.raw, "raw", "", false, "Raw values for date and size")
	flag.BoolVarP(&opts.verbose, "verbose", "v", false, "Show more information")
	flag.BoolVarP(&opts.version, "version", "", false, "Show version and exit")
	flag.StringVarP(&opts.username, "user", "u", "", "Username for authentication")
	flag.StringVarP(&opts.password, "pass", "p", "", "Password for authentication")
	flag.StringVarP(&opts.cacert, "tlscacert", "C", "", "Trust certs signed only by this CA")
	flag.StringVarP(&opts.cert, "tlscert", "c", "", "Path to TLS certificate file")
	flag.StringVarP(&opts.key, "tlskey", "k", "", "Path to TLS key file")
	flag.StringVarP(&opts.keypass, "tlskeypass", "P", "", "Passphrase for TLS key file")
	flag.StringSliceVarP(&opts.arch, "arch", "", []string{}, "Target architecture. May be specified multiple times")
	flag.StringSliceVarP(&opts.os, "os", "", []string{}, "Target OS. May be specified multiple times")
	flag.Parse()

	for _, arch := range opts.arch {
		if !slices.Contains(arches, arch) {
			log.Fatalf("Invalid arch: %s\n", arch)
		}
	}
	for _, os := range opts.os {
		if !slices.Contains(oses, os) {
			log.Fatalf("Invalid arch: %s\n", os)
		}
	}
	if len(opts.arch) > 0 || len(opts.os) > 0 {
		opts.all = true
	}

	if opts.version {
		fmt.Printf("v%s %v %s/%s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	if opts.password != "" {
		if data, err := os.ReadFile(opts.password); err == nil {
			opts.password = string(data)
		}
	}
	if opts.keypass != "" {
		if data, err := os.ReadFile(opts.keypass); err == nil {
			opts.keypass = string(data)
		}
	}

	if opts.username != "" && opts.password == "" {
		opts.password = getPass("Password: ")
	}

	if opts.cert != "" && opts.key != "" && opts.keypass == "" {
		if _, err := tls.LoadX509KeyPair(opts.cert, opts.key); err != nil {
			opts.keypass = getPass("Passphrase for %s: ", opts.key)
		}
	}

	if opts.delete {
		opts.digests = true
	}

	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(1)
	}
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
	if opts.all {
		return r.GetInfoAll(ctx, repo, ref, opts.all || opts.verbose, opts.arch, opts.os)
	}
	info, err := r.GetInfo(ctx, repo, ref, opts.verbose)
	if err != nil {
		return nil, err
	}
	return []*registry.Info{info}, err
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

func printInfo(repo string, tag string, info *registry.Info) {
	fmt.Printf("%-*s", repoWidth+20, repo+":"+tag)
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
			fmt.Printf("  %-31s", info.Created)
		} else {
			fmt.Printf("  %-31s", prettyTime(info.Created))
		}
	}
	if opts.all {
		fmt.Printf("  %-8s  %s", info.OS, info.Architecture)
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
		format := "%-20s\t%s\n"
		if info.Author != "" {
			fmt.Printf(format, "Author", info.Author)
		}
		if info.Architecture != "" {
			fmt.Printf(format, "Architecture", info.Architecture)
		}
		if info.OS != "" {
			fmt.Printf(format, "OS", info.OS)
		}
		if info.Digest != "" {
			fmt.Printf(format, "Digest", info.Digest)
		}
		fmt.Printf(format, "Id", info.ID)
		if opts.raw {
			if info.Created != "" {
				fmt.Printf(format, "Created", info.Created)
			}
			fmt.Printf("%-20s\t%d\n", "Size", info.Size)
		} else {
			if info.Created != "" {
				fmt.Printf(format, "Created", prettyTime(info.Created))
			}
			fmt.Printf(format, "Size", prettySize(info.Size))
		}

		if opts.verbose {
			for _, key := range []string{"Cmd", "Entrypoint", "Env", "ExposedPorts", "Healthcheck", "Labels", "OnBuild", "Shell", "StopSignal", "User", "Volumes", "WorkingDir"} {
				if info.Config[key] == nil {
					continue
				}
				var value string
				switch info.Config[key].(type) {
				case string:
					if info.Config[key] == "" {
						continue
					}
					value = info.Config[key].(string)
				default:
					format = "%-20s\t%s\n"
					b, _ := json.Marshal(info.Config[key])
					value = string(b)
				}
				fmt.Printf(format, key, value)
			}
			for i := range info.History {
				fmt.Printf("History[%d]\t\t%q\n", i, info.History[i]["created_by"])
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

	channels := make([]chan []string, len(repos))
	for i := range repos {
		channels[i] = make(chan []string)
	}

	go func() {
		var wg sync.WaitGroup
		wg.Add(len(repos))
		for i, repo := range repos {
			go func(repo string, channel chan []string) {
				defer wg.Done()
				tags, err := r.Tags(ctx, repo)
				if err != nil {
					log.Printf("Get tags of [%s] error: %s\n", repo, err)
				} else {
					tags = filterRegex(tags, tagRegex)
					sort.Strings(tags)
					channel <- tags
				}
				close(channel)
			}(repo, channels[i])
		}
		wg.Wait()
	}()

	printHeader()

	for i, repo := range repos {
		tags := <-channels[i]
		channels2 := make([]chan *registry.Info, len(tags))
		for i := range tags {
			channels2[i] = make(chan *registry.Info)
		}
		go func() {
			var wg sync.WaitGroup
			wg.Add(len(tags))
			for i, tag := range tags {
				go func(repo string, tag string, channel2 chan *registry.Info) {
					defer wg.Done()
					infos, err := getInfos(ctx, r, repo, tag)
					if err != nil {
						log.Printf("%s:%s: %v\n", repo, tag, err)
					} else {
						for _, info := range infos {
							channel2 <- info
						}
					}
					close(channel2)
				}(repo, tag, channels2[i])
			}
			wg.Wait()
		}()
		for i, tag := range tags {
			for info := range channels2[i] {
				printInfo(repo, tag, info)
			}
		}
	}
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

func main() {
	var domain, path string
	var repoRegex, tagRegex *regexp.Regexp
	var repoPattern, tagPattern string

	// Validate URL
	arg := flag.Args()[0]
	if !strings.HasPrefix(arg, "http:") && !strings.HasPrefix(arg, "https://") {
		arg = "https://" + arg
	}
	u, err := url.Parse(arg)
	if err != nil {
		log.Fatalf("%s: %v\n", arg, err)
	}
	domain = u.Host
	if u.Path != "" {
		path = strings.TrimPrefix(arg, u.Scheme+"://"+u.Host+"/")
		if !strings.Contains(path, "@") && strings.ContainsAny(path, "*?[") {
			v := strings.SplitN(path, ":", 2)
			repoPattern = v[0]
			if len(v) > 1 {
				tagPattern = v[1]
			}
		} else if _, err := registry.ParseImage(u.Host + u.Path); err != nil {
			log.Fatalf("%s: %v\n", arg, err)
		}
	}

	// Convert shell patterns to regular expressions
	if repoPattern != "" {
		expr, err := pattern.Regexp(repoPattern, 0)
		if err != nil {
			log.Fatalf("%s: %v\n", repoPattern, err)
		}
		repoRegex = regexp.MustCompile("^" + expr + "$")
	}
	if tagPattern != "" {
		expr, err := pattern.Regexp(tagPattern, 0)
		if err != nil {
			log.Fatalf("%s: %v\n", tagPattern, err)
		}
		tagRegex = regexp.MustCompile("^" + expr + "$")
	}

	// On ^C, or SIGTERM handle exit.
	ctx := context.WithValue(context.Background(), ContextKey(version), version)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	signal.Notify(signals, syscall.SIGTERM)
	signal.Notify(signals, syscall.SIGPIPE)
	_, cancel := context.WithCancel(ctx)
	go func() {
		for sig := range signals {
			cancel()
			log.Printf("Received %s, exiting\n", sig.String())
			os.Exit(0)
		}
	}()

	if path != "" && repoPattern == "" {
		if opts.delete {
			deleteImage(ctx, domain, path)
		} else {
			printImage(ctx, domain, path)
		}
	} else {
		if opts.delete {
			deleteAll(ctx, domain, repoRegex, tagRegex)
		} else {
			printAll(ctx, domain, repoRegex, tagRegex)
		}
	}
}
