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
	"slices"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/ricardobranco777/regview/registry"
	"mvdan.cc/sh/v3/pattern"
)

import flag "github.com/spf13/pflag"

const version = "3.0.2"

// ContextKey type for contexts
type ContextKey string

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
	format   string
	arch     []string
	os       []string
}

var (
	format              = template.New("format")
	ignoreTags          = regexp.MustCompile(`^sha(256|512)-[0-9a-f]{64,}\.(att|sig)$`) // Ignore stupid sigstore/cosign fake manifests & signatures
	repoRegex, tagRegex *regexp.Regexp
	repoWidth           int
)

func init() {
	log.SetFlags(0)
	log.SetPrefix("ERROR: ")

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
	flag.StringVarP(&opts.format, "format", "f", "", "Output format")
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
	// Filter by current arch & OS if neither --all, --arch or --os were specified
	if !opts.all && len(opts.arch) == 0 && len(opts.os) == 0 {
		opts.arch = []string{runtime.GOARCH}
		opts.os = []string{runtime.GOOS}
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
		for _, file := range []string{opts.cert, opts.key} {
			if _, err := os.ReadFile(file); err != nil {
				log.Fatalf("%s: %v", file, err)
			}
		}
		if _, err := tls.LoadX509KeyPair(opts.cert, opts.key); err != nil {
			opts.keypass = getPass("Passphrase for %s: ", opts.key)
		}
	}

	if opts.delete {
		opts.digests = true
	}

	var err error

	if opts.format != "" {
		format, err = format.Parse(opts.format)
		if err != nil {
			log.Fatal(err)
		}
		if strings.Contains(opts.format, ".Image") {
			opts.verbose = true
		}
	}

	if tz, err = time.LoadLocation("Local"); err != nil {
		log.Fatal(err)
	}
}

func main() {
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	var domain, path string
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
		printAll(ctx, domain)
	}
}
