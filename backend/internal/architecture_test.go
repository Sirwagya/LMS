package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHexagonalDependencyBoundaries(t *testing.T) {
	fset := token.NewFileSet()

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		isDomain := strings.HasPrefix(path, "domain/")
		isApp := strings.HasPrefix(path, "application/")
		if !isDomain && !isApp {
			return nil
		}

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if isDomain && strings.Contains(importPath, ".") && !strings.HasPrefix(importPath, "vedam/lms/internal/domain/") {
				t.Errorf("%s: domain layer must not import non-stdlib package %q (DIP violation)", path, importPath)
			}
			if isApp && strings.Contains(importPath, ".") && !strings.HasPrefix(importPath, "vedam/lms/internal/domain/") {
				t.Errorf("%s: application layer must depend only on stdlib and domain, found %q (DIP violation)", path, importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal packages: %v", err)
	}
}
