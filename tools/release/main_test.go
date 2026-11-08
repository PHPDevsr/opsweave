package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestVersionAndTargets(t *testing.T) {
	for _, v := range []string{"v0.1.0", "v12.2.3", "v1.0.0-rc.1", "v1.0.0-alpha-beta.0"} {
		if !validVersion(v) {
			t.Fatal(v)
		}
		for _, os := range []string{"linux", "windows", "darwin"} {
			for _, arch := range []string{"amd64", "arm64"} {
				if _, err := assetName(v, os, arch); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	for _, v := range []string{"", "dev", "1.2.3", "v01.2.3", "v1.02.3", "v1.2.03", "v1.2.3-01", "v1.2.3-rc..1", "v1.2.3+metadata", "v1.2.3;echo bad", "v1.2.3/../../x", "v1.2.3\n"} {
		if validVersion(v) {
			t.Fatal(v)
		}
	}
	if _, err := assetName("v1.0.0", "plan9", "amd64"); err == nil {
		t.Fatal("unsupported OS")
	}
	if _, err := assetName("v1.0.0", "linux", "386"); err == nil {
		t.Fatal("unsupported arch")
	}
}

func FuzzVersionFilename(f *testing.F) {
	for _, v := range []string{"v0.1.0", "v1.2.3-rc.1", "../../escape", "v1.0.0-01", "v1.2.3\n"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) {
		name, err := assetName(v, "windows", "arm64")
		if err == nil && (!validVersion(v) || strings.ContainsAny(name, "/\\\r\n\x00")) {
			t.Fatal("unsafe archive name", name)
		}
	})
}

func TestArchivesAreReproducibleAndExecutable(t *testing.T) {
	for _, goos := range []string{"linux", "darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			var a, b bytes.Buffer
			if err := writeArchive(&a, goos, []byte("binary"), []byte("instructions")); err != nil {
				t.Fatal(err)
			}
			if err := writeArchive(&b, goos, []byte("binary"), []byte("instructions")); err != nil || !bytes.Equal(a.Bytes(), b.Bytes()) {
				t.Fatal("non-deterministic archive", err)
			}
			got := map[string]string{}
			if goos == "windows" {
				r, err := zip.NewReader(bytes.NewReader(a.Bytes()), int64(a.Len()))
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range r.File {
					entry, err := file.Open()
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(entry)
					if err := errors.Join(err, entry.Close()); err != nil {
						t.Fatal(err)
					}
					got[file.Name] = string(data)
				}
			} else {
				r, err := gzip.NewReader(bytes.NewReader(a.Bytes()))
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := r.Close(); err != nil {
						t.Error(err)
					}
				}()
				archive := tar.NewReader(r)
				for {
					header, err := archive.Next()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatal(err)
					}
					data, err := io.ReadAll(archive)
					if err != nil {
						t.Fatal(err)
					}
					got[header.Name] = string(data)
					if header.Name == "opsweave" && header.Mode != 0755 {
						t.Fatal("binary not executable")
					}
				}
			}
			binary := "opsweave"
			if goos == "windows" {
				binary += ".exe"
			}
			if len(got) != 2 || got[binary] != "binary" || got["INSTALL.md"] != "instructions" {
				t.Fatal(got)
			}
		})
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestArchiveErrorsPropagate(t *testing.T) {
	for _, os := range []string{"linux", "windows"} {
		if err := writeArchive(brokenWriter{}, os, []byte("binary"), nil); err == nil {
			t.Fatal("write failure hidden")
		}
	}
}
