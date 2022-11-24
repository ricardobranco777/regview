package main

import (
	"context"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/tls"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"

	"github.com/ricardobranco777/regview/registry"
	"github.com/ricardobranco777/regview/repoutils"
	"golang.org/x/exp/slices"
	"mvdan.cc/sh/v3/pattern"
)

import flag "github.com/spf13/pflag"

const version = "2.9"

// ContextKey type for contexts
type ContextKey string

var maxWorkers = 100

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
		fmt.Printf("v%s %v %s/%s %s\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH, getCommit())
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
		infos, err = r.GetInfoAll(ctx, repo, ref, opts.all || opts.verbose, opts.arch, opts.os)
		if err != nil {
			return []*registry.Info{}, err
		}
		return infos, nil
	}
	info, err := r.GetInfo(ctx, repo, ref, opts.verbose)
	if err != nil {
		return []*registry.Info{}, err
	}
	return []*registry.Info{info}, err
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
