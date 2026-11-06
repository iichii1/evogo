package group_service

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Criar grupo e alterar participantes mandam as pessoas em nó bruto. Com o "+" que
// utils.ParseJID coloca em número solto, o servidor não responde e a chamada expira
// (ensaio de 2026-11-06), então nenhuma das duas pode ler o número direto.
func TestGroupOperationsReadParticipantsWithoutThePlusSign(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "group_service.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || (function.Name.Name != "CreateGroup" && function.Name.Name != "UpdateParticipant") || function.Body == nil {
			continue
		}
		found[function.Name.Name] = true
		parsesDirectly, usesParser := false, false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch callee := call.Fun.(type) {
			case *ast.SelectorExpr:
				if callee.Sel.Name == "ParseJID" {
					parsesDirectly = true
				}
			case *ast.Ident:
				if callee.Name == "parseRequestParticipants" {
					usesParser = true
				}
			}
			return true
		})
		if parsesDirectly || !usesParser {
			t.Errorf("%s: deve ler as pessoas com parseRequestParticipants, não com utils.ParseJID", function.Name.Name)
		}
	}
	if !found["CreateGroup"] || !found["UpdateParticipant"] {
		t.Fatalf("funções não encontradas: %v", found)
	}
}
