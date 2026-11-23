package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestProductionLogCallsUseEnglish(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, dir := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isLogCall(call) {
					return true
				}
				ast.Inspect(call, func(arg ast.Node) bool {
					switch x := arg.(type) {
					case *ast.BasicLit:
						if x.Kind == token.STRING {
							value, e := strconv.Unquote(x.Value)
							if e == nil && containsJapanese(value) {
								t.Errorf("%s: Japanese string in log call: %q", path, value)
							}
						}
					case *ast.SelectorExpr:
						if id, ok := x.X.(*ast.Ident); ok && id.Name == "i18n" && x.Sel.Name == "JA" {
							t.Errorf("%s: i18n.JA in log call", path)
						}
					}
					return true
				})
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func isLogCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Info", "Warn", "Error", "Debug", "Log":
		return true
	}
	return false
}

func containsJapanese(s string) bool {
	for _, r := range s {
		if r >= 0x3000 && r <= 0x30ff || r >= 0x3400 && r <= 0x9fff || r >= 0xff00 && r <= 0xffef {
			return true
		}
	}
	return false
}
