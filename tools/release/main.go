// Command release builds a versioned archive without third-party packaging dependencies.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

func main() {
	version := flag.String("version", "", "Git release tag, such as v0.1.0-rc.1")
	out := flag.String("out", "dist", "Output directory")
	goos := flag.String("goos", runtime.GOOS, "Target OS")
	arch := flag.String("goarch", runtime.GOARCH, "Target architecture")
	validate := flag.Bool("validate-only", false, "Validate the tag without building")
	flag.Parse()
	if flag.NArg() != 0 || !validVersion(*version) {
		fmt.Fprintln(os.Stderr, "release requires vMAJOR.MINOR.PATCH[-PRERELEASE] without build metadata")
		os.Exit(1)
	}
	if *validate {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := build(ctx, *version, *out, *goos, *arch); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func validVersion(v string) bool {
	if len(v) > 128 {
		return false
	}
	re := regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
	m := re.FindStringSubmatch(v)
	if m == nil {
		return false
	}
	for _, part := range strings.Split(m[4], ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return false
		}
	}
	return true
}

func assetName(version, goos, arch string) (string, error) {
	if !validVersion(version) || (arch != "amd64" && arch != "arm64") {
		return "", errors.New("unsupported release version or architecture")
	}
	suffix := ".tar.gz"
	switch goos {
	case "linux", "darwin":
	case "windows":
		suffix = ".zip"
	default:
		return "", errors.New("unsupported release OS")
	}
	return "opsweave_" + version + "_" + goos + "_" + arch + suffix, nil
}

func build(ctx context.Context, version, out, goos, arch string) (err error) {
	name, err := assetName(version, goos, arch)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0755); err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	temp, err := os.MkdirTemp(out, ".package-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(temp)) }()
	binaryName := "opsweave"
	if goos == "windows" {
		binaryName += ".exe"
	}
	binaryPath := filepath.Join(temp, binaryName)
	command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid= -X github.com/PHPDevsr/opsweave/internal/cli.version="+version, "-o", binaryPath, "./cmd/opsweave")
	command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+arch)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("build release binary: %w", err)
	}
	binary, err := os.ReadFile(binaryPath)
	if err != nil {
		return err
	}
	install, err := os.ReadFile("docs/install.md")
	if err != nil {
		return err
	}
	path := filepath.Join(out, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	hash := sha256.New()
	archiveErr := writeArchive(io.MultiWriter(file, hash), goos, binary, install)
	if err := errors.Join(archiveErr, file.Close()); err != nil {
		return err
	}
	checksum, err := os.OpenFile(path+".sha256", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, writeErr := fmt.Fprintf(checksum, "%x  %s\n", hash.Sum(nil), name)
	return errors.Join(writeErr, checksum.Close())
}

func writeArchive(w io.Writer, goos string, binary, install []byte) error {
	names := []string{"opsweave", "INSTALL.md"}
	data := [][]byte{binary, install}
	modes := []int64{0755, 0644}
	stamp := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	if goos == "windows" {
		names[0] += ".exe"
		archive := zip.NewWriter(w)
		for i, name := range names {
			header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: stamp}
			header.SetMode(os.FileMode(modes[i]))
			entry, err := archive.CreateHeader(header)
			if err != nil {
				return errors.Join(err, archive.Close())
			}
			if _, err := entry.Write(data[i]); err != nil {
				return errors.Join(err, archive.Close())
			}
		}
		return archive.Close()
	}
	compressed := gzip.NewWriter(w)
	archive := tar.NewWriter(compressed)
	for i, name := range names {
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: modes[i], Size: int64(len(data[i])), ModTime: stamp, Format: tar.FormatUSTAR}); err != nil {
			return errors.Join(err, archive.Close(), compressed.Close())
		}
		if _, err := archive.Write(data[i]); err != nil {
			return errors.Join(err, archive.Close(), compressed.Close())
		}
	}
	return errors.Join(archive.Close(), compressed.Close())
}
