package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestTargetEnvironmentReplacesCrossCompileVariables(t *testing.T) {
	t.Parallel()
	result := targetEnvironment([]string{
		"PATH=example", "goos=old", "GOARCH=old", "CGO_ENABLED=1",
	}, target{goos: "linux", goarch: "arm64"})
	want := []string{"PATH=example", "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64"}
	if len(result) != len(want) {
		t.Fatalf("environment = %v", result)
	}
	for index := range want {
		if result[index] != want[index] {
			t.Fatalf("environment = %v", result)
		}
	}
}

func TestArchiveIsDeterministicAndPreservesLinuxExecutableMode(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "package")
	linuxWorker := filepath.Join(root, "worker", "linux-amd64", "dnd-engine")
	if err := os.MkdirAll(filepath.Dir(linuxWorker), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linuxWorker, []byte("worker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "addon.json"), []byte("{}\n"), 0o640); err != nil {
		t.Fatal(err)
	}

	first := filepath.Join(t.TempDir(), "first.zip")
	second := filepath.Join(t.TempDir(), "second.zip")
	if err := createArchive(root, first); err != nil {
		t.Fatal(err)
	}
	if err := createArchive(root, second); err != nil {
		t.Fatal(err)
	}
	firstBody, _ := os.ReadFile(first)
	secondBody, _ := os.ReadFile(second)
	if !bytes.Equal(firstBody, secondBody) {
		t.Fatal("identical package trees produced different archives")
	}

	archive, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, entry := range archive.File {
		if entry.Name == "worker/linux-amd64/dnd-engine" {
			if entry.Mode().Perm()&0o111 == 0 {
				t.Fatalf("linux worker mode = %o", entry.Mode().Perm())
			}
			return
		}
	}
	t.Fatal("linux worker is absent from the archive")
}
