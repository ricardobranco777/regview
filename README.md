![Build Status](https://github.com/ricardobranco777/xhash/actions/workflows/ci.yml/badge.svg)

# regview

View the contents of a Docker Registry v2

Docker image available at `ghcr.io/ricardobranco777/regview:latest`

## Usage

```
regview [OPTIONS] REGISTRY[/REPOSITORY[:TAG|@DIGEST]]
  -a, --all                 Print information for all architecture
      --arch strings        Target architecture. May be specified multiple times
      --debug               Enable debug
      --delete              Delete images. USE WITH CAUTION
      --digests             Show digests
      --dry-run             Used with --delete: only show the images that would be deleted
  -f, --format string       Output format
      --insecure            Allow insecure server connections
      --no-trunc            Don't truncate output
      --os strings          Target OS. May be specified multiple times
  -p, --pass string         Password for authentication
      --raw                 Raw values for date and size
  -C, --tlscacert string    Trust certs signed only by this CA
  -c, --tlscert string      Path to TLS certificate file
  -k, --tlskey string       Path to TLS key file
  -P, --tlskeypass string   Passphrase for TLS key file
  -u, --user string         Username for authentication
  -v, --verbose             Show more information
      --version             Show version and exit
Valid options for --arch: 386 amd64 arm arm64 mips mips64 mips64le mipsle ppc64 ppc64le riscv64 s390x wasm
Valid options for --os: aix android darwin dragonfly freebsd illumos ios js linux netbsd openbsd plan9 solaris windows
```

## Notes

- Shell pattern matching is supported in repositories and tags like `busybo?/late*` or `debian:[7-9]`

## Supported authentication methods

- HTTP Basic Authentication
- Token Authentication

## Supported registries

- [Docker Distribution](https://github.com/distribution/distribution)
- [Amazon ECR](https://aws.amazon.com/blogs/compute/authenticating-amazon-ecr-repositories-for-docker-cli-with-credential-helper/) (get credentials with `aws ecr get-login` and run `docker login`)
- [Azure ACR](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-faq) (get credentials with `az acr credential show -n $` and run `docker login`)
- [Google GCR](https://cloud.google.com/container-registry/docs/advanced-authentication) (run `gcloud auth configure-docker` and use `[ZONE.]gcr.io/<PROJECT>/*` to list the registry)

## Deleting images

To delete tagged images you can use the `--delete` option.  Use the `--dry-run` option is you want to view the images that would be deleted.

Steps:
1. Make sure that the registry container has the `REGISTRY_STORAGE_DELETE_ENABLED` environment variable (or the relevant entry in `/etc/docker/registry/config.yml`) set to `true`.
1. Run `regview --delete ...`
1. Either stop or restart the registry cointainer in maintenance mode by setting the `REGISTRY_STORAGE_MAINTENANCE_READONLY` environment variable to `true` (or editing the relevant entry in `/etc/docker/registry/config.yml`).
1. Run `docker run --rm --volumes-from $CONTAINER registry:2 garbage-collect /etc/docker/registry/config.yml` if the container was stopped. Otherwise `docker exec $CONTAINER garbage-collect /etc/docker/registry/config.yml` if the container is in maintenance mode.
1. Optionally run the same command from above appending `--delete-untagged` to delete untagged images.
1. Restart the registry container in production mode.

NOTES:
- The `--delete-untagged` option was added to Docker Registry 2.7.0
- The `--delete-untagged` option is [BUGGY](https://github.com/distribution/distribution/issues/3178) with multi-arch images. The only workaround is to  push those images adding the os/arch name to the image name.
- USE AT YOUR OWN RISK!

## BUGS / LIMITATIONS

- Listing a pull through cache Registry may pollute the cache with unwanted images as the cache proxies requests, ending up with `TOOMANYREQUESTS error: "You have reached your pull rate limit. You may increase the limit by authenticating and upgrading: https://www.docker.com/increase-rate-limit".`
- When developing this tool I was warned that I was DDOS'ing the production registry, so be careful when tweaking the code that uses goroutines.  This [commit](https://github.com/ricardobranco777/regview/commit/3c827e88940056f005387f8b5b13315d5b0e1e5f) may have led to the discovery of [CVE-2023-2253](https://bugzilla.suse.com/show_bug.cgi?id=1207705)
