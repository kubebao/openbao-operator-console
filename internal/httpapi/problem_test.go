package httpapi

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/kubebao/openbao-operator-console/internal/contract"
)

func TestProblemResponsesFollowRFC9457(t *testing.T) {
	recorder := httptest.NewRecorder()
	problem(recorder, http.StatusConflict, "conflict", "Settings changed. Reload before saving.")
	if got := recorder.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("content type = %q", got)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"type":   "about:blank",
		"title":  "Conflict",
		"status": float64(http.StatusConflict),
		"detail": "Settings changed. Reload before saving.",
		"code":   "conflict",
	}
	if len(body) != len(want) {
		t.Fatalf("body = %v", body)
	}
	for key, value := range want {
		if body[key] != value {
			t.Fatalf("%s = %v, want %v", key, body[key], value)
		}
	}
}

// The contract enum must list exactly the codes the handlers emit: a literal outside the
// enum would break clients that switch on it, and an unused member documents a failure
// that cannot happen.
func TestProblemCodesMatchTheContract(t *testing.T) {
	emitted := emittedProblemCodes(t)
	declared := declaredProblemCodes(t)
	for code := range emitted {
		if !contract.ProblemCode(code).Valid() {
			t.Errorf("code %q is emitted but not in the contract", code)
		}
	}
	for _, code := range declared {
		if !emitted[code] {
			t.Errorf("contract code %q is never emitted", code)
		}
	}
}

// emittedProblemCodes collects string literals passed as the code of problem calls and
// problemError values in the package's non-test sources.
func emittedProblemCodes(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	emitted := map[string]bool{}
	record := func(expression ast.Expr) {
		literal, ok := expression.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatal(err)
		}
		emitted[value] = true
	}
	fileSet := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CallExpr:
				if function, ok := node.Fun.(*ast.Ident); ok && function.Name == "problem" && len(node.Args) == 4 {
					record(node.Args[2])
				}
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == "code" {
					record(node.Value)
				}
			}
			return true
		})
	}
	if len(emitted) == 0 {
		t.Fatal("no problem codes found")
	}
	return emitted
}

// declaredProblemCodes reads the ProblemCode constants from the generated contract.
func declaredProblemCodes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("..", "contract", "generated.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	declared := []string{}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok || group.Tok != token.CONST {
			continue
		}
		for _, spec := range group.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Values) != 1 {
				continue
			}
			if kind, ok := value.Type.(*ast.Ident); !ok || kind.Name != "ProblemCode" {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			code, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			declared = append(declared, code)
		}
	}
	if len(declared) == 0 {
		t.Fatal("no contract problem codes found")
	}
	return declared
}
