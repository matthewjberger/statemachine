package statemachine

import (
	"strings"
	"testing"
)

func mustParseForValidate(t *testing.T, source string) *AST {
	t.Helper()
	ast, err := Parse("x.sm", []byte(source))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ast
}

func TestValidateDuplicateTransition(t *testing.T) {
	ast := mustParseForValidate(t, `
package: main
transitions: {
    *A + Go = B,
    A + Go = C,
}
`)
	err := Validate("x.sm", ast)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate transition") {
		t.Errorf("error %q", err)
	}
}

func TestValidateMultipleInitial(t *testing.T) {
	ast := mustParseForValidate(t, `
package: main
transitions: {
    *A + Go = B,
    *B + Stop = A,
}
`)
	err := Validate("x.sm", ast)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "multiple initial states") {
		t.Errorf("error %q", err)
	}
}

func TestValidateDuplicateWildcard(t *testing.T) {
	ast := mustParseForValidate(t, `
package: main
transitions: {
    *A + Go = B,
    _ + Reset = A,
    _ + Reset = B,
}
`)
	err := Validate("x.sm", ast)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "duplicate wildcard") {
		t.Errorf("error %q", err)
	}
}

func TestValidateOK(t *testing.T) {
	ast := mustParseForValidate(t, `
package: main
transitions: {
    *A + Go = B,
    B + Stop = A,
    _ + Reset = A,
}
`)
	if err := Validate("x.sm", ast); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
