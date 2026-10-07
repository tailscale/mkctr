// Copyright (c) 2026 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

func TestArchiveBuild(t *testing.T) {
	// Use an empty Docker config so host credential helpers are not called.
	configDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", configDir)

	for _, mode := range []string{"image", "index", "variants", "publish"} {
		t.Run(mode, func(t *testing.T) {
			var logs bytes.Buffer
			oldOutput := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(oldOutput)
			var writes atomic.Int32
			handler := registry.New()
			reg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" && r.Method != "HEAD" && strings.HasPrefix(r.URL.Path, "/v2/built/") {
					writes.Add(1)
				}
				handler.ServeHTTP(w, r)
			}))
			defer reg.Close()
			repo := strings.TrimPrefix(reg.URL, "http://")
			base, err := name.NewTag(repo+"/base:latest", name.Insecure)
			if err != nil {
				t.Fatal(err)
			}
			arch := "amd64"
			platforms := []v1.Platform{{
				OS:           "linux",
				Architecture: arch,
			}}
			if mode == "variants" {
				arch = "arm"
				platforms = []v1.Platform{
					{OS: "linux", Architecture: arch, Variant: "v6"},
					{OS: "linux", Architecture: arch, Variant: "v7"},
				}
			}
			var adds []mutate.IndexAddendum
			for _, p := range platforms {
				img, err := mutate.ConfigFile(empty.Image, &v1.ConfigFile{
					OS:           p.OS,
					Architecture: p.Architecture,
					Variant:      p.Variant,
				})
				if err != nil {
					t.Fatal(err)
				}
				adds = append(adds, mutate.IndexAddendum{
					Add: img,
					Descriptor: v1.Descriptor{
						Platform: &p,
					},
				})
			}
			if mode == "image" {
				err = remote.Write(base, adds[0].Add.(v1.Image))
			} else {
				err = remote.WriteIndex(base, mutate.AppendManifests(empty.Index, adds...))
			}
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			staticFile := filepath.Join(dir, "file")
			if err := os.WriteFile(staticFile, []byte("test content"), 0644); err != nil {
				t.Fatal(err)
			}
			bp := &buildParams{
				baseImage:   base.String(),
				staticFiles: map[string]string{staticFile: "/srv/file"},
				goarch:      []string{arch},
				output:      filepath.Join(dir, "image.oci.tar"),
				annotations: map[string]string{"test.annotation": "test value"},
			}
			built, err := name.NewTag(repo+"/built:test", name.Insecure)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "publish" {
				bp.publish = true
				bp.imageRefs = []name.Tag{built}
			}
			if err := fetchAndBuild(bp); err != nil {
				t.Fatal(err)
			}
			lp := readArchive(t, bp.output)
			idx, err := lp.ImageIndex()
			if err != nil {
				t.Fatal(err)
			}
			manifest, err := idx.IndexManifest()
			if err != nil || len(manifest.Manifests) != 1 {
				t.Fatalf("archive index = %+v, %v", manifest, err)
			}
			root := manifest.Manifests[0]
			if mode == "variants" {
				idx, err = idx.ImageIndex(root.Digest)
				if err != nil {
					t.Fatal(err)
				}
				manifest, err = idx.IndexManifest()
				if err != nil || len(manifest.Manifests) != 2 {
					t.Fatalf("variant index = %+v, %v", manifest, err)
				}
				if manifest.Annotations["test.annotation"] != "test value" {
					t.Fatalf("index annotations = %+v", manifest.Annotations)
				}
				if !strings.Contains(logs.String(), "index digest: "+root.Digest.String()) {
					t.Fatalf("logged digest does not match archived index %v:\n%s", root.Digest, logs.String())
				}
			}
			for i, desc := range manifest.Manifests {
				if desc.Platform == nil || !reflect.DeepEqual(*desc.Platform, platforms[i]) {
					t.Fatalf("platform = %+v; want %+v", desc.Platform, platforms[i])
				}
				img, err := idx.Image(desc.Digest)
				if err != nil {
					t.Fatal(err)
				}
				im, err := img.Manifest()
				if err != nil || im.Annotations["test.annotation"] != "test value" {
					t.Fatalf("image annotations = %+v, %v", im, err)
				}
				layers, err := img.Layers()
				if err != nil || len(layers) != 1 {
					t.Fatalf("layers = %v, %v", layers, err)
				}
				r, err := layers[0].Uncompressed()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(r)
				r.Close()
				if err != nil || !bytes.Contains(data, []byte("test content")) {
					t.Fatalf("missing file content in layer: %v", err)
				}
			}
			if mode == "publish" {
				desc, err := remote.Get(built)
				if err != nil || desc.Digest != root.Digest {
					t.Fatalf("published image does not match archive: %v", err)
				}
				if writes.Load() == 0 {
					t.Fatal("no registry writes with --push")
				}
			} else if writes.Load() != 0 {
				t.Fatal("archive-only build wrote to the output registry")
			}
			bp.goarch = []string{"unsupported"}
			bp.output = filepath.Join(dir, "missing.oci.tar")
			if err := fetchAndBuild(bp); err == nil {
				t.Fatal("build succeeded with no matching architecture")
			}
			if _, err := os.Stat(bp.output); !os.IsNotExist(err) {
				t.Fatalf("failed build produced output: %v", err)
			}
		})
	}
}

func readArchive(t *testing.T, path string) layout.Path {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dir := t.TempDir()
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsLocal(h.Name) || h.Typeflag != tar.TypeReg {
			t.Fatalf("unexpected archive entry: %+v", h)
		}
		path := filepath.Join(dir, h.Name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	lp, err := layout.FromPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lp
}

func TestArchiveFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.oci.tar")
	const original = "original content"
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("failed to write image")
	err := writeArchive(path, func(layout.Path) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v; want %v", err, wantErr)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != original {
		t.Fatalf("failed export changed existing file: %q, %v", got, err)
	}
	if err := writeIndexArchive(filepath.Join(path, "invalid.oci.tar"), empty.Index); err == nil {
		t.Fatal("write succeeded with invalid output path")
	}
}
