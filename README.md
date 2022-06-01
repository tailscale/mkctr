# `mkctr`: cross platform container builder for go

`mkctr` is a small go binary which uses `GOOS= GOARCH= go build` directly to compile go binaries and then uses [go-containerregistry](https://github.com/google/go-containerregistry) to create and publish the new containers based on the desired platforms.

This is inspired by [ko](https://github.com/google/ko) which is awesome but doesn't support multiple binaries in a single container.

## Usage

```bash
mkctr \
  --base="ghcr.io/tailscale/alpine-base:3.14" \
  --gopaths="\
    tailscale.com/cmd/tailscale:/usr/local/bin/tailscale, \
    tailscale.com/cmd/tailscaled:/usr/local/bin/tailscaled" \
  --tags="latest" \
  --repos="tailscale/tailscale" \
  [--target=<target>] \ # e.g. flyio
  [--push] \
  [--] [<cmd>...]
```

`mkctr` auto discovers `GOOS`/`GOARCH` from the specified base image. If the base image supports multiple platforms, binaries are compiled for each platform as long as it's one of `linux/amd64`, `linux/386`, `linux/arm`, `linux/arm64`.

## Maturity
This is under active development. While Tailscale uses it, backwards compatability is not guaranteed, and some functionality is missing.