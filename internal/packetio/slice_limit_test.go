package packetio

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

// TestCheckSliceLengthIgnoresWrites pins the encode-path guarantee: a writer
// is never rejected, however large the slice it is writing.
func TestCheckSliceLengthIgnoresWrites(t *testing.T) {
	var buffer bytes.Buffer
	CheckSliceLength(protocol.NewWriter(&buffer, 0), 1<<20)
}

// TestCheckSliceLengthRejectsTruncatedRead proves the guard still fires when a
// decoder asks for more elements than the payload could hold.
func TestCheckSliceLengthRejectsTruncatedRead(t *testing.T) {
	reader := protocol.NewReader(bytes.NewBuffer(make([]byte, 4)), 0, true)
	defer func() {
		if recover() == nil {
			t.Fatal("oversized slice length was accepted")
		}
	}()
	CheckSliceLength(reader, 100)
}

// TestCheckSliceLengthCallsAreReadGated keeps the read-path invariant in the
// repo: every call site must sit inside an `if io.reading` block. The helper
// itself also no-ops on writers, so this is a second, visible lock on the same
// property rather than the only one.
func TestCheckSliceLengthCallsAreReadGated(t *testing.T) {
	fset := token.NewFileSet()
	var files []string
	err := filepath.WalkDir(filepath.Join("..", "..", "protocols"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	gated := make(map[*ast.CallExpr]bool)
	var calls []*ast.CallExpr
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			statement, ok := node.(*ast.IfStmt)
			if !ok || !mentionsReading(statement.Cond) {
				return true
			}
			ast.Inspect(statement.Body, func(inner ast.Node) bool {
				if call, ok := inner.(*ast.CallExpr); ok && isCheckSliceLength(call) {
					gated[call] = true
				}
				return true
			})
			return true
		})
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok && isCheckSliceLength(call) {
				calls = append(calls, call)
			}
			return true
		})
	}
	if len(calls) == 0 {
		t.Fatal("no CheckSliceLength call sites found")
	}
	for _, call := range calls {
		if !gated[call] {
			t.Fatalf("%s: CheckSliceLength is not inside an if io.reading block", fset.Position(call.Pos()))
		}
	}
}

func isCheckSliceLength(call *ast.CallExpr) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	return ok && selector.Sel.Name == "CheckSliceLength"
}

func mentionsReading(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == "reading" {
			found = true
		}
		return true
	})
	return found
}
