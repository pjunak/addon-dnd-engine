package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

var raceTargets = []string{"./internal/rules", "./internal/provider", "./internal/engine"}

var buildDependencyPins = []string{"host-sdk-revision.txt"}

func main() {
	command := "all"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "all":
		fast()
		test()
	case "fast":
		fast()
	case "test":
		test()
	case "vuln":
		run("go", "tool", "-modfile=go.tools.mod", "govulncheck", "./...")
	case "workflows":
		workflows()
	case "dependency-ref":
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "use dependency-ref <revision-file>")
			os.Exit(2)
		}
		revision, err := readBuildDependencyRevision(os.Args[2])
		must(err)
		fmt.Println(revision)
	case "format":
		files := goFiles()
		if len(files) > 0 {
			run("gofmt", append([]string{"-w"}, files...)...)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown check %q; use fast, test, vuln, workflows, dependency-ref, format, or all\n", command)
		os.Exit(2)
	}
}
func fast() {
	for _, pin := range buildDependencyPins {
		_, err := readBuildDependencyRevision(pin)
		must(err)
	}
	checkFormatting(goFiles())
	run("go", "vet", "./...")
	// Engine errors are complete user-facing sentences returned through the
	// service contract, so ST1005's library-error style is intentionally inapplicable.
	run("go", "tool", "-modfile=go.tools.mod", "staticcheck", "-checks=all,-ST1005", "./...")
}
func checkFormatting(files []string) {
	unformatted, err := unformattedFiles(files)
	must(err)
	if len(unformatted) > 0 {
		fmt.Fprintf(os.Stderr, "Go files need formatting:\n%s\n", strings.Join(unformatted, "\n"))
		os.Exit(1)
	}
}

func unformattedFiles(files []string) ([]string, error) {
	var unformatted []string
	for _, file := range files {
		source, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		source = bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
		cmd := exec.Command("gofmt")
		cmd.Stdin = bytes.NewReader(source)
		formatted, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(source, formatted) {
			unformatted = append(unformatted, file)
		}
	}
	return unformatted, nil
}
func test() {
	run("go", "test", "./...")
	run("go", append([]string{"test", "-race"}, raceTargets...)...)
}
func goFiles() []string {
	raw, err := gitOutput("ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "*.go")
	must(err)
	var files []string
	for _, file := range strings.Split(raw, "\x00") {
		if file == "" {
			continue
		}
		// Git still lists tracked files deleted before their removal is staged.
		_, err := os.Stat(file)
		if os.IsNotExist(err) {
			continue
		}
		must(err)
		files = append(files, file)
	}
	sort.Strings(files)
	return files
}
func workflows() {
	var files []string
	must(filepath.WalkDir(".github/workflows", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".yml") || strings.HasSuffix(path, ".yaml")) {
			files = append(files, filepath.ToSlash(path))
		}
		return nil
	}))
	sort.Strings(files)
	// actionlint 1.7.12 predates GitHub's queue property; keep all other validation.
	args := []string{"tool", "-modfile=go.tools.mod", "actionlint", "-ignore", `unexpected key "queue" for "concurrency" section`}
	run("go", append(args, files...)...)
}

// Resolve pins with the standard library so CI can run this before fetching
// the local module replacements. Mutable refs cannot identify a commit release.
func readBuildDependencyRevision(path string) (string, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read build dependency pin %q: %w", path, err)
	}
	revision := strings.TrimSpace(string(source))
	valid := len(revision) == 40
	for _, character := range revision {
		valid = valid && (character >= '0' && character <= '9' || character >= 'a' && character <= 'f')
	}
	if !valid {
		return "", fmt.Errorf("build dependency pin %q must contain one full lowercase commit SHA", path)
	}
	return revision, nil
}

func gitOutput(args ...string) (string, error) { return output("git", args...) }
func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return stdout.String(), err
}
func run(name string, args ...string) {
	fmt.Printf("+ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	must(cmd.Run())
}
func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
