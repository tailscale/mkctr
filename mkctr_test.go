// Copyright (c) 2021 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestLayerFromFilesReproducible(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		src := filepath.Join(dir, name)
		if err := os.WriteFile(src, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		files[src] = "/usr/local/bin/" + name
	}
	logf := func(string, ...any) {}

	var first string
	for i := range 30 {
		layer, err := layerFromFiles(logf, files, types.OCILayer)
		if err != nil {
			t.Fatal(err)
		}
		d, err := layer.DiffID()
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = d.String()
			continue
		}
		if d.String() != first {
			t.Fatalf("build %d produced layer %s; build 0 produced %s; want identical layers for identical inputs", i, d, first)
		}
	}
}
