// Copyright (c) 2026 Tailscale Inc & AUTHORS All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/layout"
)

func writeImageArchive(path string, img v1.Image, platform v1.Platform) error {
	return writeArchive(path, func(lp layout.Path) error {
		return lp.AppendImage(img, layout.WithPlatform(platform))
	})
}

func writeIndexArchive(path string, idx v1.ImageIndex) error {
	return writeArchive(path, func(lp layout.Path) error {
		return lp.AppendIndex(idx)
	})
}

// writeArchive writes an OCI image layout as a tar archive. The output path
// is replaced only after the archive is complete.
func writeArchive(path string, appendContent func(layout.Path) error) error {
	dir, err := os.MkdirTemp("", "mkctr-output-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	lp, err := layout.Write(dir, empty.Index)
	if err != nil {
		return err
	}
	if err := appendContent(lp); err != nil {
		return fmt.Errorf("writing OCI layout: %w", err)
	}
	out, err := os.CreateTemp(filepath.Dir(path), ".mkctr-output-*")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	tw := tar.NewWriter(out)
	// Use fixed tar metadata for reproducible archives. AddFS would copy
	// timestamps, permissions, and ownership from the host filesystem.
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Name: filepath.ToSlash(rel),
			Mode: 0644,
			Size: info.Size(),
		}); err != nil {
			return err
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(tw, in)
		return err
	})
	if closeErr := tw.Close(); err == nil {
		err = closeErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(out.Name(), path)
}
