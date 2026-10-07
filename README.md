# `mkctr`: cross platform container builder for go

`mkctr` is a small go binary which uses `GOOS= GOARCH= go build` directly to compile go binaries and then uses [go-containerregistry](https://github.com/google/go-containerregistry) to create and publish the new containers based on the desired platforms.

This is inspired by [ko](https://github.com/google/ko) which is awesome but doesn't support multiple binaries in a single container.

## Usage

```bash
mkctr \
  --base="alpine:latest" \
  --gopaths="\
    tailscale.com/cmd/tailscale:/usr/local/bin/tailscale, \
    tailscale.com/cmd/tailscaled:/usr/local/bin/tailscaled" \
  --tags="latest" \
  --repos="tailscale/tailscale" \
  [--files=foo.txt:/var/lib/foo.txt,bar.txt:/var/lib/bar.txt] \
  [--target=<target>] \ # e.g. flyio, local
  [--user=1000:1000] \ # user (uid[:gid]) to run the container as
  [--push] \
  [--output=image.oci.tar] \
  [--] [<cmd>...]
```

### Output modes

Every mode requires `--base` and at least one of `--gopaths` or `--files`.
`--repos` and `--tags` accept comma-separated lists; when required, both must
be set. Each repository receives each tag when publishing or loading locally.

| Mode | Flags | Result |
| --- | --- | --- |
| Build only (default) | `--repos` and `--tags`, without `--push` or `--output` | Builds the image but does not publish, load, or save it. |
| Archive only | `--output=image.oci.tar`, without `--push` | Writes an OCI image layout tar archive. `--repos` and `--tags` are optional, but if either is set, both are required. |
| Publish | `--push`, `--repos`, and `--tags` | Pushes the image or multi-platform index to the specified registries. |
| Load locally | `--target=local`, `--push`, `--repos`, and `--tags` | Builds for the host architecture and loads the image into the local Docker daemon under the specified image references. |

`--output` can also be combined with `--push` to save an archive and publish
or load the same image. The archive is written before publishing or loading.
Without `--push`, `--target=local` only selects the host architecture; it does
not load the image into Docker.

Archives contain an [OCI image layout](https://github.com/opencontainers/image-spec/blob/main/image-layout.md),
with an `oci-layout` file, an `index.json`, and content-addressed blobs.

A single selected platform is stored as an image; multiple selected platforms
are stored as an image index. For format details, see the OCI
[image manifest](https://github.com/opencontainers/image-spec/blob/main/manifest.md)
and [image index](https://github.com/opencontainers/image-spec/blob/main/image-index.md)
specifications. Archives do not include tags from `--repos` or `--tags`.
Archive-only builds still need access to the base image registry, but do not
write to a registry or need a Docker daemon.

For example, to save an archive without publishing:

```bash
mkctr \
  --base="alpine:latest" \
  --gopaths="./cmd/server:/usr/local/bin/server" \
  --output="server.oci.tar"
```

To save the same archive and publish it, add `--push`,
`--repos="example.com/my/server"`, and `--tags="latest"`.

### Container configuration

By default the container runs as the base image's user (for most base images,
root). Use `--user` to set the `User` in the image config, e.g.
`--user=1000:1000` to run as UID 1000, GID 1000. Note that the image's
filesystem is not otherwise modified: files and directories added by `mkctr`
are owned by root, so a non-root `--user` can only write to world-writable
paths (such as `/tmp`) or volumes mounted at runtime.

`mkctr` auto discovers `GOOS`/`GOARCH` from the specified base image. If the base image supports multiple platforms, binaries are compiled for each platform as long as it's one of `linux/amd64`, `linux/386`, `linux/arm`, `linux/arm64`. Multi-arch base image must be either an [OCI image index](https://github.com/opencontainers/image-spec/blob/main/image-index.md) or [Docker manifest list](https://github.com/openshift/docker-distribution/blob/master/docs/spec/manifest-v2-2.md#manifest-list).
`mkctr` produces image of the same media type as the base image and uses the media type of the base image, or of the individual image references in case of a multi-arch image, to determine the media type of the layer it builds.

## Maturity
This is under active development. While Tailscale uses it, backwards compatability is not guaranteed, and some functionality is missing.
