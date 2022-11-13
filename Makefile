# Setup name variables for the package/tool
NAME := regview
PKG := github.com/ricardobranco777/$(NAME)

CGO_ENABLED := 0

# Set any default go build tags.
BUILDTAGS :=

include basic.mk

.PHONY: prebuild
prebuild:


