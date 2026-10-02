package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDependencyPinsRequireImmutableCommits(t *testing.T) {
	for _, test := range []struct {
		name  string
		value string
		valid bool
	}{
		{"full commit", strings.Repeat("abcdef01", 5) + "\n", true},
		{"Windows newline", strings.Repeat("abcdef01", 5) + "\r\n", true},
		{"branch", "main\n", false},
		{"short commit", "abcdef01\n", false},
		{"nonhex", strings.Repeat("g", 40), false},
		{"uppercase", strings.Repeat("A", 40), false},
		{"multiple commits", strings.Repeat("a", 40) + "\n" + strings.Repeat("b", 40), false},
		{"workflow output injection", strings.Repeat("a", 40) + "\nother=value", false},
		{"empty", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pin := filepath.Join(t.TempDir(), "revision.txt")
			if err := os.WriteFile(pin, []byte(test.value), 0o600); err != nil {
				t.Fatal(err)
			}
			revision, err := readBuildDependencyRevision(pin)
			if test.valid {
				if err != nil || revision != strings.TrimSpace(test.value) {
					t.Fatalf("immutable revision = %q, %v", revision, err)
				}
			} else if err == nil {
				t.Fatalf("mutable or malformed revision accepted: %q", revision)
			}
		})
	}
	if _, err := readBuildDependencyRevision(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("missing pin accepted")
	}
}

func TestDependencyResolverBootstrapsWithoutSiblingModules(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "tools"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner, err := os.ReadFile("check.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tools", "check.go"), runner, 0o600); err != nil {
		t.Fatal(err)
	}
	module := "module fixture\n\ngo 1.27.1\n\nrequire (\n github.com/pjunak/ttrpg-codex v0.0.0\n github.com/pjunak/addon-dnd-engine v0.0.0\n)\n\nreplace github.com/pjunak/ttrpg-codex => ../missing-host\nreplace github.com/pjunak/addon-dnd-engine => ../missing-engine\n"
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte(module), 0o600); err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("abcdef01", 5)
	pin := filepath.Join(root, "host-sdk-revision.txt")
	if err := os.WriteFile(pin, []byte(revision+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolve := func() ([]byte, error) {
		command := exec.Command("go", "run", "./tools/check.go", "dependency-ref", "host-sdk-revision.txt")
		command.Dir = root
		command.Env = append(os.Environ(), "GOFLAGS=-mod=readonly", "GOWORK=off", "GOPROXY=off", "GOSUMDB=off")
		return command.CombinedOutput()
	}
	result, err := resolve()
	if err != nil || string(result) != revision+"\n" {
		t.Fatalf("offline bootstrap = %q, %v", result, err)
	}
	if err := os.WriteFile(pin, []byte("main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if result, err := resolve(); err == nil {
		t.Fatalf("bootstrap silently accepted a branch: %q", result)
	}
}
