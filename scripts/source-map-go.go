package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"slices"
	"strings"
)

type symbol struct {
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Signature string   `json:"signature"`
	Line      int      `json:"line"`
	Calls     []string `json:"calls,omitempty"`
	Comment   string   `json:"comment,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var paths []string
	if err := json.NewDecoder(os.Stdin).Decode(&paths); err != nil {
		return err
	}
	result := make(map[string][]symbol, len(paths))
	for _, path := range paths {
		symbols, err := readSymbols(path)
		if err != nil {
			return err
		}
		result[path] = symbols
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func readSymbols(path string) ([]symbol, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	positions := token.NewFileSet()
	file, err := parser.ParseFile(positions, path, source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	line := func(pos token.Pos) int { return positions.PositionFor(pos, false).Line }
	text := func(start, end token.Pos) string {
		return strings.TrimSpace(string(source[positions.PositionFor(start, false).Offset:positions.PositionFor(end, false).Offset]))
	}
	symbols := []symbol{}
	for _, decl := range file.Decls {
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			name := decl.Name.Name
			if decl.Recv != nil {
				receiver := decl.Recv.List[0].Type
				name = text(receiver.Pos(), receiver.End()) + "." + name
			}
			end := decl.End()
			if decl.Body != nil {
				end = decl.Body.Pos()
			}
			symbols = append(symbols, symbol{name, "Function", text(decl.Pos(), end), line(decl.Pos()), callsIn(decl.Body), decl.Doc.Text()})
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				typ, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				symbols = append(symbols, symbol{Name: typ.Name.Name, Kind: "Type", Signature: "type " + text(typ.Pos(), typ.End()), Line: line(typ.Pos()), Comment: typ.Doc.Text()})
				if iface, ok := typ.Type.(*ast.InterfaceType); ok {
					for _, method := range iface.Methods.List {
						for _, name := range method.Names {
							symbols = append(symbols, symbol{Name: typ.Name.Name + "." + name.Name, Kind: "Signature", Signature: text(method.Pos(), method.End()), Line: line(method.Pos())})
						}
					}
				}
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		if fn, ok := node.(*ast.FuncLit); ok {
			at := line(fn.Pos())
			symbols = append(symbols, symbol{Name: fmt.Sprintf("func @%d", at), Kind: "Closure", Signature: text(fn.Pos(), fn.Body.Pos()), Line: at, Calls: callsIn(fn.Body)})
		}
		return true
	})
	slices.SortStableFunc(symbols, func(a, b symbol) int { return a.Line - b.Line })
	return symbols, nil
}

func callsIn(body *ast.BlockStmt) []string {
	if body == nil {
		return nil
	}
	var calls []string
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if name := callName(call.Fun); name != "" && !slices.Contains(calls, name) {
				calls = append(calls, name)
			}
		}
		return true
	})
	return calls
}

func callName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.SelectorExpr:
		if receiver := callName(expr.X); receiver != "" {
			return receiver + "." + expr.Sel.Name
		}
		return expr.Sel.Name
	case *ast.IndexExpr:
		return callName(expr.X)
	case *ast.IndexListExpr:
		return callName(expr.X)
	case *ast.ParenExpr:
		return callName(expr.X)
	default:
		return ""
	}
}
