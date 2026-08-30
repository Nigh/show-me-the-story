package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestCanonicalWritePrimitivesRemainAllowlisted(t *testing.T) {
	wantedIdentifiers := map[string]bool{
		"writeFileAtomic": true,
		"writeFile":       true,
		"deleteFile":      true,
		"renameFile":      true,
	}
	wantedOSSelectors := map[string]bool{
		"WriteFile": true,
		"Remove":    true,
		"RemoveAll": true,
		"Rename":    true,
	}

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	actualSet := map[string]bool{}
	fileSet := token.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				callee := ""
				switch expression := call.Fun.(type) {
				case *ast.Ident:
					if wantedIdentifiers[expression.Name] {
						callee = expression.Name
					}
				case *ast.SelectorExpr:
					packageName, ok := expression.X.(*ast.Ident)
					if ok && packageName.Name == "os" && wantedOSSelectors[expression.Sel.Name] {
						callee = "os." + expression.Sel.Name
					}
				}
				if callee != "" {
					actualSet[filepath.Base(path)+":"+function.Name.Name+":"+callee] = true
				}
				return true
			})
		}
	}

	actual := sortedKeys(actualSet)
	expected := []string{
		"agent.go:getBuiltinTools:deleteFile",
		"chat.go:DeleteChatSession:deleteFile",
		"chat.go:SaveChatSession:writeFileAtomic",
		"chat.go:saveChatSessions:writeFileAtomic",
		"config.go:saveAPIConfig:writeFileAtomic",
		"config.go:saveConfig:writeFileAtomic",
		"config_guard.go:SavePendingConfigChanges:deleteFile",
		"config_guard.go:SavePendingConfigChanges:writeFileAtomic",
		"filesys.go:deleteFileImpl:os.Remove",
		"filesys.go:renameFileImpl:os.Rename",
		"filesys.go:writeFileImpl:os.WriteFile",
		"foreshadow.go:SaveForeshadowRoadmap:os.WriteFile",
		"handlers.go:DeleteChaptersFrom:deleteFile",
		"handlers.go:DeleteProgress:deleteFile",
		"handlers.go:GetChatSessions:deleteFile",
		"handlers.go:PostChatSession:writeFileAtomic",
		"handlers.go:PutAPIConfig:writeFileAtomic",
		"handlers.go:PutConfig:writeFileAtomic",
		"handlers.go:writeFileAtomic:deleteFile",
		"handlers.go:writeFileAtomic:renameFile",
		"handlers.go:writeFileAtomic:writeFile",
		"postprocess.go:SavePostProcess:writeFileAtomic",
		"settings.go:SaveProjectSettings:writeFileAtomic",
		"state.go:SaveChapterMarkdown:os.WriteFile",
		"state.go:SaveProgress:writeFileAtomic",
		"web.go:DeleteProject:os.RemoveAll",
		"writing_delete.go:clearChapterContentAt:deleteFile",
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("canonical write surface changed\nactual:   %q\nexpected: %q", actual, expected)
	}
}

func sortedKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
