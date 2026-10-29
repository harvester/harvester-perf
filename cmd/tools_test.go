package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	body     string
	linkname string
}

func makeTarGz(t *testing.T, entries []tarEntry) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		hdr := &tar.Header{
			Name:     e.name,
			Typeflag: e.typeflag,
			Mode:     0o755,
			Size:     int64(len(e.body)),
			Linkname: e.linkname,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf
}

func TestInstallTools(t *testing.T) {
	t.Run("extracts regular files with their mode", func(t *testing.T) {
		dir := t.TempDir()
		archive := makeTarGz(t, []tarEntry{
			{name: "benchmark", typeflag: tar.TypeReg, body: "bench"},
			{name: "etcdctl", typeflag: tar.TypeReg, body: "ctl"},
		})
		if err := installTools(archive, dir); err != nil {
			t.Fatalf("installTools() error = %v", err)
		}

		for name, want := range map[string]string{"benchmark": "bench", "etcdctl": "ctl"} {
			filePath := filepath.Join(dir, name)
			got, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != want {
				t.Errorf("%s content = %q, want %q", name, got, want)
			}

			fi, err := os.Stat(filePath)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm()&0o100 == 0 {
				t.Errorf("%s is not executable: %v", name, fi.Mode())
			}
		}
	})

	t.Run("overwrites existing tools", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "benchmark")
		if err := os.WriteFile(filePath, []byte("old-and-longer"), 0o755); err != nil {
			t.Fatal(err)
		}
		archive := makeTarGz(t, []tarEntry{{name: "benchmark", typeflag: tar.TypeReg, body: "new"}})
		if err := installTools(archive, dir); err != nil {
			t.Fatalf("installTools() error = %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "new" {
			t.Errorf("content = %q, want %q", got, "new")
		}
	})

	t.Run("skips non-regular entries", func(t *testing.T) {
		dir := t.TempDir()
		archive := makeTarGz(t, []tarEntry{
			{name: "subdir/", typeflag: tar.TypeDir},
			{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
			{name: "benchmark", typeflag: tar.TypeReg, body: "bench"},
		})
		if err := installTools(archive, dir); err != nil {
			t.Fatalf("installTools() error = %v", err)
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "benchmark" {
			t.Errorf("got entries %v, want only [benchmark]", entries)
		}
	})

	t.Run("rejects paths escaping the tools dir", func(t *testing.T) {
		parent := t.TempDir()
		dir := filepath.Join(parent, "tools")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}

		archive := makeTarGz(t, []tarEntry{{name: "../escaped", typeflag: tar.TypeReg, body: "x"}})
		if err := installTools(archive, dir); err == nil {
			t.Error("installTools() error = nil, want error for path traversal")
		}

		if _, err := os.Stat(filepath.Join(parent, "escaped")); !os.IsNotExist(err) {
			t.Errorf("file escaped the tools dir, stat err = %v", err)
		}
	})

	t.Run("invalid gzip", func(t *testing.T) {
		if err := installTools(bytes.NewBufferString("not a gzip"), t.TempDir()); err == nil {
			t.Error("installTools() error = nil, want error for invalid gzip")
		}
	})

	t.Run("missing tools dir", func(t *testing.T) {
		archive := makeTarGz(t, []tarEntry{{name: "benchmark", typeflag: tar.TypeReg, body: "x"}})
		if err := installTools(archive, filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Error("installTools() error = nil, want error for missing tools dir")
		}
	})
}
