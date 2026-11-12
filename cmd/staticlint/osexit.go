package main

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// OsExitAnalyzer — собственный анализатор, запрещающий os.Exit в main.main.
var OsExitAnalyzer = &analysis.Analyzer{
	Name: "osexit",
	Doc:  "restricts direct calls to os.Exit in main function of main package",
	Run:  runOsExit,
}

func runOsExit(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	for _, file := range pass.Files {
		filename := pass.Fset.File(file.Pos()).Name()
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}

		ast.Inspect(file, func(node ast.Node) bool {
			funcDecl, ok := node.(*ast.FuncDecl)
			if !ok {
				return true
			}

			if funcDecl.Recv != nil || funcDecl.Name.Name != "main" || funcDecl.Body == nil {
				return true
			}

			ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
				callExpr, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				selExpr, ok := callExpr.Fun.(*ast.SelectorExpr)
				if !ok || selExpr.Sel.Name != "Exit" {
					return true
				}

				ident, ok := selExpr.X.(*ast.Ident)
				if !ok {
					return true
				}

				if obj, ok := pass.TypesInfo.Uses[ident].(*types.PkgName); ok && obj.Imported().Path() == "os" {
					pass.Reportf(callExpr.Pos(), "direct call to os.Exit in main function is prohibited")
				}

				return true
			})

			return false
		})
	}

	return nil, nil
}
