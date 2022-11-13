# Setup name variables for the package/tool
NAME := regview
PKG := github.com/ricardobranco777/$(NAME)

CGO_ENABLED := 0

# Set the shell
SHELL := /bin/bash

# Set an output prefix, which is the local directory if not specified
PREFIX?=$(shell pwd)

# Set the build dir, where built cross-compiled binaries will be output
BUILDDIR := ${PREFIX}/cross

# Set our default go compiler
GO := go

# List the GOOS and GOARCH to build
GOOSARCHES = $(shell cat .goosarch)

.PHONY: build
build: prebuild $(NAME) ## Builds a dynamic executable or package.

$(NAME): $(wildcard *.go) $(wildcard */*.go)
	@echo "+ $@"
	$(GO) build ${GO_LDFLAGS} -o $(NAME) .

.PHONY: static
static: prebuild ## Builds a static executable.
	@echo "+ $@"
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build ${GO_LDFLAGS_STATIC} -o $(NAME) .

all: clean build fmt lint test staticcheck vet install ## Runs a clean, build, fmt, lint, test, staticcheck, vet and install.

.PHONY: fmt
fmt: ## Verifies all files have been `gofmt`ed.
	@echo "+ $@"
	@if [[ ! -z "$(shell gofmt -s -l . | tee /dev/stderr)" ]]; then \
		exit 1; \
	fi

.PHONY: lint
lint: ## Verifies `golint` passes.
	@echo "+ $@"
	@if [[ ! -z "$(shell golint ./... | tee /dev/stderr)" ]]; then \
		exit 1; \
	fi

.PHONY: test
test: prebuild ## Runs the go tests.
	@echo "+ $@"
	@$(GO) test -v $(shell $(GO) list ./...)

.PHONY: vet
vet: ## Verifies `go vet` passes.
	@echo "+ $@"
	@if [[ ! -z "$(shell $(GO) vet $(shell $(GO) list ./...) | tee /dev/stderr)" ]]; then \
		exit 1; \
	fi

.PHONY: staticcheck
staticcheck: ## Verifies `staticcheck` passes.
	@echo "+ $@"
	@if [[ ! -z "$(shell staticcheck $(shell $(GO) list ./...) | tee /dev/stderr)" ]]; then \
		exit 1; \
	fi

.PHONY: cover
cover: prebuild ## Runs go test with coverage.
	@echo "" > coverage.txt
	@for d in $(shell $(GO) list ./...); do \
		$(GO) test -race -coverprofile=profile.out -covermode=atomic "$$d"; \
		if [ -f profile.out ]; then \
			cat profile.out >> coverage.txt; \
			rm profile.out; \
		fi; \
	done;

.PHONY: install
install: prebuild ## Installs the executable or package.
	@echo "+ $@"
	$(GO) install -a -tags "$(BUILDTAGS)" ${GO_LDFLAGS} .

define buildpretty
mkdir -p $(BUILDDIR)/$(1)/$(2);
GOOS=$(1) GOARCH=$(2) CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
	 -o $(BUILDDIR)/$(1)/$(2)/$(NAME) \
	 -a -tags "$(BUILDTAGS) static_build netgo" \
	 -installsuffix netgo ${GO_LDFLAGS_STATIC} .;
md5sum $(BUILDDIR)/$(1)/$(2)/$(NAME) > $(BUILDDIR)/$(1)/$(2)/$(NAME).md5;
sha256sum $(BUILDDIR)/$(1)/$(2)/$(NAME) > $(BUILDDIR)/$(1)/$(2)/$(NAME).sha256;
endef

.PHONY: cross
cross: *.go prebuild ## Builds the cross-compiled binaries, creating a clean directory structure (eg. GOOS/GOARCH/binary).
	@echo "+ $@"
	$(foreach GOOSARCH,$(GOOSARCHES), $(call buildpretty,$(subst /,,$(dir $(GOOSARCH))),$(notdir $(GOOSARCH))))

define buildrelease
GOOS=$(1) GOARCH=$(2) CGO_ENABLED=$(CGO_ENABLED) $(GO) build \
	 -o $(BUILDDIR)/$(NAME)-$(1)-$(2) \
	 -a -tags "$(BUILDTAGS) static_build netgo" \
	 -installsuffix netgo ${GO_LDFLAGS_STATIC} .;
md5sum $(BUILDDIR)/$(NAME)-$(1)-$(2) > $(BUILDDIR)/$(NAME)-$(1)-$(2).md5;
sha256sum $(BUILDDIR)/$(NAME)-$(1)-$(2) > $(BUILDDIR)/$(NAME)-$(1)-$(2).sha256;
endef

.PHONY: release
release: *.go prebuild ## Builds the cross-compiled binaries, naming them in such a way for release (eg. binary-GOOS-GOARCH).
	@echo "+ $@"
	$(foreach GOOSARCH,$(GOOSARCHES), $(call buildrelease,$(subst /,,$(dir $(GOOSARCH))),$(notdir $(GOOSARCH))))

.PHONY: image
image: ## Create the docker image from the Dockerfile.
	@docker build --rm --force-rm -t $(NAME) .

.PHONY: clean
clean: ## Cleanup any build binaries or packages.
	@echo "+ $@"
	$(RM) $(NAME)
	$(RM) -r $(BUILDDIR)
	$(RM) -f tests/certs/*

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed 's/^[^:]*://g' | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: prebuild
prebuild:
