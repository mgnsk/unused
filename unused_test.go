package main_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

func TestUnused(t *testing.T) {
	exe := buildExecutable(t)

	mode := packages.LoadImports | packages.NeedSyntax
	cfg := &packages.Config{
		Mode:  mode,
		Dir:   "testdata",
		Tests: true,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) == 0 {
		t.Fatalf("no packages found")
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatalf("packages contain errors")
	}

	type entry struct {
		content string
		file    string
		line    int
	}

	// Collect want comments from testdata.
	var wants []entry
	packages.Visit(pkgs, func(pkg *packages.Package) bool {
		for _, f := range pkg.Syntax {
			for _, group := range f.Comments {
				for _, comment := range group.List {
					if strings.HasPrefix(comment.Text, "// want ") {
						trimmed := strings.TrimPrefix(comment.Text, "// want ")
						content, err := strconv.Unquote(trimmed)
						if err != nil {
							t.Fatal(err)
						}

						pos := pkg.Fset.Position(comment.Pos())
						wants = append(wants, entry{
							content: content,
							file:    pos.Filename,
							line:    pos.Line,
						})
					}
				}
			}
		}
		return true
	}, nil)

	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)

	// Run the command.
	cmd := exec.Command(exe, "-exclude-files", "*.excluded.go", "-exclude-objects", "*Exclude")
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Dir = "testdata"
	cmd.Env = append(os.Environ(), "GOPROXY=", "GO111MODULE=on")

	if err := cmd.Run(); err == nil {
		t.Fatalf("expected error")
	}

	fmt.Println(stderr.String())

	// Collect actual command output.
	var gots []entry
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		file, remaining, _ := strings.Cut(scanner.Text(), ":")
		line, remaining, _ := strings.Cut(remaining, ":")
		_, content, _ := strings.Cut(remaining, " ")
		lineNr, err := strconv.Atoi(line)
		if err != nil {
			t.Fatal(err)
		}
		gots = append(gots, entry{
			content: content,
			file:    file,
			line:    lineNr,
		})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}

	for _, want := range wants {
		if !slices.Contains(gots, want) {
			t.Logf("want %v, got no error", want)
			t.Fail()
		}
	}

	for _, got := range gots {
		if !slices.Contains(wants, got) {
			t.Logf("got %v, wanted no error", got)
			t.Fail()
		}
	}
}

func buildExecutable(t *testing.T) string {
	bin := filepath.Join(t.TempDir(), "unused")
	cmd := exec.Command("go", "build", "-o", bin)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Building unused: %v\n%s", err, out)
	}
	return bin
}
