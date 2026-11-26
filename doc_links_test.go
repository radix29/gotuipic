package gotuipic

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A deleted document's pointers outlive it: the review plan removed in 33c9683
// stayed cited from README.md, CLAUDE.md and ARCHITECTURE.md. This walks every .md file and every Go comment for a `*.md` path and
// fails on one that does not resolve, so the next deletion is caught at the
// deletion rather than at the next review. (Ported from gossms's
// TestNoDanglingDocReference.)
var (
	docPathRe = regexp.MustCompile(`[A-Za-z0-9_][A-Za-z0-9_./-]*\.md`)
	urlRe     = regexp.MustCompile(`https?://\S+`)
)

func TestNoDanglingDocReference(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	exists := func(p string) bool {
		_, err := os.Stat(p)
		return err == nil
	}
	check := func(rel, ref string, line int) {
		// From the repo root, or relative to the citing file's directory.
		if exists(filepath.Join(root, ref)) || exists(filepath.Join(root, filepath.Dir(rel), ref)) {
			return
		}
		t.Errorf("%s:%d: references %s, which does not exist — replace the pointer with the "+
			"fact it was pointing at, in the document that now owns it", rel, line, ref)
	}
	scan := func(rel, text string, line int) {
		for i, l := range strings.Split(urlRe.ReplaceAllString(text, " "), "\n") {
			for _, ref := range docPathRe.FindAllString(l, -1) {
				check(rel, ref, line+i)
			}
		}
	}

	fset := token.NewFileSet()
	checkedMD, checkedGo := 0, 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(path, ".md"):
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			checkedMD++
			scan(rel, string(src), 1)
		case strings.HasSuffix(path, ".go"):
			f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			checkedGo++
			for _, group := range f.Comments {
				for _, c := range group.List {
					scan(rel, c.Text, fset.Position(c.Pos()).Line)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkedMD == 0 || checkedGo == 0 {
		t.Fatalf("walked %d .md and %d .go files; the root is wrong and this test proves nothing",
			checkedMD, checkedGo)
	}
}
