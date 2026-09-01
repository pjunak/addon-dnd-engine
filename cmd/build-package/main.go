// Command build-package cross-compiles the native worker and creates the
// deterministic add-on archive consumed by the host package manager.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type target struct {
	goos   string
	goarch string
	path   string
}

type manifestIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type checksumInventory struct {
	Algorithm string            `json:"algorithm"`
	Files     map[string]string `json:"files"`
}

var targets = []target{
	{goos: "windows", goarch: "amd64", path: "worker/windows-amd64/dnd-engine.exe"},
	{goos: "linux", goarch: "amd64", path: "worker/linux-amd64/dnd-engine"},
	{goos: "linux", goarch: "arm64", path: "worker/linux-arm64/dnd-engine"},
}

var packageEntries = []string{"addon.json", "contracts", "LICENSE", "README.md", "worker"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	repositoryRoot, err := findRepositoryRoot()
	if err != nil {
		return err
	}
	if err := buildWorkers(repositoryRoot); err != nil {
		return err
	}

	identity, err := readManifest(repositoryRoot)
	if err != nil {
		return err
	}
	distributionRoot := filepath.Join(repositoryRoot, "dist")
	packageRoot := filepath.Join(distributionRoot, "package")
	archivePath := filepath.Join(distributionRoot, identity.ID+"-"+identity.Version+".zip")
	if err := os.RemoveAll(packageRoot); err != nil {
		return fmt.Errorf("remove old package directory: %w", err)
	}
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove old package archive: %w", err)
	}
	if err := os.MkdirAll(packageRoot, 0o750); err != nil {
		return fmt.Errorf("create package directory: %w", err)
	}
	for _, name := range packageEntries {
		if err := copyEntry(filepath.Join(repositoryRoot, name), filepath.Join(packageRoot, name)); err != nil {
			return err
		}
	}
	if err := writeChecksums(packageRoot); err != nil {
		return err
	}
	if err := createArchive(packageRoot, archivePath); err != nil {
		return err
	}
	relative, _ := filepath.Rel(repositoryRoot, archivePath)
	fmt.Println(filepath.ToSlash(relative))
	return nil
}

func findRepositoryRoot() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("locate build command source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "addon.json")); err != nil {
		return "", fmt.Errorf("locate repository root: %w", err)
	}
	return root, nil
}

func buildWorkers(root string) error {
	goExecutable := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goExecutable += ".exe"
	}
	for _, item := range targets {
		output := filepath.Join(root, filepath.FromSlash(item.path))
		if err := os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
			return fmt.Errorf("create %s output directory: %w", item.path, err)
		}
		command := exec.Command(goExecutable,
			"build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=",
			"-o", output, "./cmd/worker",
		)
		command.Dir = root
		command.Env = targetEnvironment(os.Environ(), item)
		command.Stdout = os.Stdout
		command.Stderr = os.Stderr
		if err := command.Run(); err != nil {
			return fmt.Errorf("build %s: %w", item.path, err)
		}
		mode := fs.FileMode(0o644)
		if item.goos != "windows" {
			mode = 0o755
		}
		if err := os.Chmod(output, mode); err != nil {
			return fmt.Errorf("set %s permissions: %w", item.path, err)
		}
	}
	return nil
}

func targetEnvironment(environment []string, item target) []string {
	filtered := make([]string, 0, len(environment)+3)
	for _, value := range environment {
		key, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(key) {
		case "CGO_ENABLED", "GOOS", "GOARCH":
			continue
		default:
			filtered = append(filtered, value)
		}
	}
	return append(filtered, "CGO_ENABLED=0", "GOOS="+item.goos, "GOARCH="+item.goarch)
}

func readManifest(root string) (manifestIdentity, error) {
	body, err := os.ReadFile(filepath.Join(root, "addon.json"))
	if err != nil {
		return manifestIdentity{}, fmt.Errorf("read manifest: %w", err)
	}
	var identity manifestIdentity
	if err := json.Unmarshal(body, &identity); err != nil || identity.ID == "" || identity.Version == "" {
		return manifestIdentity{}, errors.New("manifest package identity is invalid")
	}
	return identity, nil
}

func copyEntry(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("inspect package entry %s: %w", source, err)
	}
	if !info.IsDir() {
		return copyFile(source, destination, info.Mode())
	}
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o750)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("package source contains unsupported entry %s", path)
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, targetPath, entryInfo.Mode())
	})
}

func copyFile(source, destination string, mode fs.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open package source %s: %w", source, err)
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode.Perm())
	if err != nil {
		return fmt.Errorf("create package file %s: %w", destination, err)
	}
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = os.Remove(destination)
		}
	}()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy package file %s: %w", destination, err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close package file %s: %w", destination, err)
	}
	complete = true
	return nil
}

func writeChecksums(root string) error {
	files, err := packageFiles(root)
	if err != nil {
		return err
	}
	inventory := checksumInventory{Algorithm: "sha256", Files: make(map[string]string, len(files))}
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		inventory.Files[name] = hex.EncodeToString(digest[:])
	}
	body, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.WriteFile(filepath.Join(root, "checksums.json"), body, 0o640); err != nil {
		return fmt.Errorf("write checksum inventory: %w", err)
	}
	return nil
}

func createArchive(root, destination string) error {
	files, err := packageFiles(root)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return fmt.Errorf("create package archive: %w", err)
	}
	complete := false
	defer func() {
		_ = output.Close()
		if !complete {
			_ = os.Remove(destination)
		}
	}()
	archive := zip.NewWriter(output)
	fixedTime := time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	for _, name := range files {
		mode := fs.FileMode(0o644)
		if strings.HasPrefix(name, "worker/linux-") {
			mode = 0o755
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixedTime)
		header.SetMode(mode)
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create archive entry %s: %w", name, err)
		}
		input, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return fmt.Errorf("write archive entry %s: %w", name, copyErr)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finish package archive: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close package archive: %w", err)
	}
	complete = true
	return nil
}

func packageFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("package contains unsupported entry %s", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	sort.Strings(files)
	return files, err
}
