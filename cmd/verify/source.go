// Source checks enforce the repository's size, documentation and safety rules.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const maxFileLines = 600
const maxFunctionLines = 50
const maxParameters = 5

func sourceChecks() error {
	return filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != "." && strings.HasPrefix(entry.Name(), ".") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		return checkFile(path)
	})
}

func checkFile(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Count(source, []byte{'\n'}) > maxFileLines {
		return fmt.Errorf("%s: R-SIZE-2: more than 600 lines", path)
	}
	formatted, err := format.Source(source)
	if err != nil {
		return err
	}
	if !bytes.Equal(formatted, source) {
		return fmt.Errorf("%s: R-STYLE-1: run gofmt", path)
	}
	if !bytes.HasPrefix(source, []byte("//")) {
		return fmt.Errorf("%s: R-DOC-6: file needs a purpose comment", path)
	}
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, source, parser.ParseComments)
	if err != nil {
		return err
	}
	for _, imported := range file.Imports {
		if imported.Path.Value == `"unsafe"` {
			return fmt.Errorf("%s: R-SAFE-1: unsafe is forbidden", path)
		}
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if err := checkFunction(function, positions, source); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}

func checkFunction(function *ast.FuncDecl, positions *token.FileSet, source []byte) error {
	name := function.Name.Name
	if function.Body == nil {
		return nil
	}
	start, end := positions.Position(function.Body.Pos()).Offset, positions.Position(function.Body.End()).Offset
	lines := codeLines(string(source[start:end]))
	if lines > maxFunctionLines {
		return fmt.Errorf("%s: R-SIZE-1: %d code lines, limit 50", name, lines)
	}
	parameters := 0
	for _, field := range function.Type.Params.List {
		parameters += max(1, len(field.Names))
	}
	if parameters > maxParameters {
		return fmt.Errorf("%s: R-SIZE-3: %d parameters, limit 5", name, parameters)
	}
	if ast.IsExported(name) && function.Doc == nil {
		return fmt.Errorf("%s: R-DOC-1: missing documentation", name)
	}
	test := strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Fuzz")
	if test && !strings.Contains(function.Doc.Text(), "Verifies: [") {
		return fmt.Errorf("%s: R-TRACE-1: missing Verifies paragraph", name)
	}
	return nil
}

func codeLines(body string) int {
	count := 0
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || line == "{" || line == "}" {
			continue
		}
		count++
	}
	return count
}
