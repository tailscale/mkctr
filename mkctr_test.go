// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestLayerFromFilesMissingSource(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	files := map[string]string{missing: "/app/file"}
	logf := func(string, ...any) {}

	_, err := layerFromFiles(logf, files, types.OCILayer)
	if err == nil {
		t.Fatal("layerFromFiles succeeded for a source path that does not exist")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error = %q; want it to name the missing path", err)
	}
}

func TestLayerFromFilesCopiesFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	logf := func(string, ...any) {}

	layer, err := layerFromFiles(logf, map[string]string{dir: "/app"}, types.OCILayer)
	if err != nil {
		t.Fatal(err)
	}
	rc, err := layer.Uncompressed()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var got []string
	tr := tar.NewReader(rc)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, h.Name)
	}
	if !slices.Contains(got, "/app/a.txt") {
		t.Errorf("layer entries = %q; want /app/a.txt", got)
	}
}
