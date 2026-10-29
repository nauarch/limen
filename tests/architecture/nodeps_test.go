// Copyright 2026 darki73
// SPDX-License-Identifier: Apache-2.0

package architecture_test

// Verifies:
//   - limen depends on nothing outside the standard library: go.mod requires no module and every
//     Go file imports only the standard library and limen itself

import (
	"strings"
	"testing"
)

// dependencyProblems lists every required module and every import from outside the standard
// library and the module.
func dependencyProblems(t *testing.T, tr tree) []string {
	mod := tr.modulePath(t)
	var problems []string
	for line := range strings.SplitSeq(string(tr["go.mod"]), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "require") {
			problems = append(problems, "go.mod: requires a module ("+strings.TrimSpace(line)+")")
		}
	}
	for _, f := range tr.parseGo(t, tr.paths(isGo)) {
		for _, imp := range imports(f.file) {
			if !isStd(imp) && imp != mod && !strings.HasPrefix(imp, mod+"/") {
				problems = append(problems, f.path+": imports "+imp)
			}
		}
	}
	return problems
}

func TestNothingOutsideTheStandardLibraryIsUsed(t *testing.T) {
	expectNone(t, dependencyProblems(t, repo(t)))
}

func TestAPlantedDependencyIsCaught(t *testing.T) {
	planted := tree{
		"go.mod":  []byte(goModFor + "\nrequire github.com/x/y v1.0.0\n"),
		"a/a.go":  []byte("package a\n\n// A uses y.\nimport \"github.com/x/y\"\n"),
		"b/b.go":  []byte("package b\n\n// B uses its own.\nimport (\n\t\"net/http\"\n\t\"example.com/m/internal/c\"\n)\n"),
		"go.sum":  nil,
		"README":  nil,
		"x/x.txt": nil,
	}
	expectCaught(t, dependencyProblems(t, planted), "go.mod: requires a module", "a/a.go: imports github.com/x/y")
}
