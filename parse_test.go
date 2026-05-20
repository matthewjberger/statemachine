package statemachine

import (
	"strings"
	"testing"
)

func mustParse(t *testing.T, source string) *AST {
	t.Helper()
	ast, err := Parse("test.sm", []byte(source))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return ast
}

func TestParseMinimal(t *testing.T) {
	ast := mustParse(t, `
package: main
transitions: {
    *Idle + Start = Running,
    Running + Stop = Idle,
}
`)
	if ast.Package != "main" {
		t.Errorf("Package = %q, want main", ast.Package)
	}
	if ast.Name != "" {
		t.Errorf("Name = %q, want empty", ast.Name)
	}
	if got, want := len(ast.Transitions), 2; got != want {
		t.Fatalf("transitions = %d, want %d", got, want)
	}
	first := ast.Transitions[0]
	if !first.Sources.States[0].Initial {
		t.Errorf("first source state not marked initial")
	}
	if first.Sources.States[0].Name != "Idle" {
		t.Errorf("first source = %q, want Idle", first.Sources.States[0].Name)
	}
	if first.Target.State != "Running" {
		t.Errorf("first target = %q, want Running", first.Target.State)
	}
}

func TestParseName(t *testing.T) {
	ast := mustParse(t, `
package: foo
name: Player
transitions: {
    *Idle + Move = Walking,
}
`)
	if ast.Name != "Player" {
		t.Errorf("Name = %q, want Player", ast.Name)
	}
}

func TestParseStatePattern(t *testing.T) {
	ast := mustParse(t, `
package: main
transitions: {
    *Idle | Waiting + Start = Active,
}
`)
	sources := ast.Transitions[0].Sources.States
	if len(sources) != 2 {
		t.Fatalf("sources = %d, want 2", len(sources))
	}
	if !sources[0].Initial || sources[1].Initial {
		t.Errorf("initial flags = %v, %v; want true, false", sources[0].Initial, sources[1].Initial)
	}
	if sources[0].Name != "Idle" || sources[1].Name != "Waiting" {
		t.Errorf("source names = %q, %q; want Idle, Waiting", sources[0].Name, sources[1].Name)
	}
}

func TestParseEventPattern(t *testing.T) {
	ast := mustParse(t, `
package: main
transitions: {
    *Active + Pause | Stop = Idle,
}
`)
	events := ast.Transitions[0].Events
	if len(events) != 2 || events[0] != "Pause" || events[1] != "Stop" {
		t.Errorf("events = %v, want [Pause Stop]", events)
	}
}

func TestParseWildcardSource(t *testing.T) {
	ast := mustParse(t, `
package: main
transitions: {
    *A + Go = B,
    _ + Reset = A,
}
`)
	if !ast.Transitions[1].Sources.Wildcard {
		t.Errorf("second transition not flagged as wildcard")
	}
}

func TestParseInternalTarget(t *testing.T) {
	ast := mustParse(t, `
package: main
transitions: {
    *Moving + Tick = _,
    Moving + Arrive = Idle,
}
`)
	if !ast.Transitions[0].Target.Internal {
		t.Errorf("first transition not flagged as internal target")
	}
}

func TestParseLineComments(t *testing.T) {
	mustParse(t, `
// header comment
package: main // trailing comment
transitions: {
    // inside the block
    *Idle + Start = Running, // trailing on transition
}
`)
}

func TestParseTrailingCommaOptional(t *testing.T) {
	mustParse(t, `
package: main
transitions: {
    *Idle + Start = Running,
    Running + Stop = Idle
}
`)
}

func TestParseErrorMissingPackage(t *testing.T) {
	_, err := Parse("x.sm", []byte(`transitions: { *A + B = C, }`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing required header: package") {
		t.Errorf("error %q does not mention missing package", err)
	}
}

func TestParseErrorUnknownHeader(t *testing.T) {
	_, err := Parse("x.sm", []byte(`bogus: thing
package: main
transitions: { *A + B = C, }`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown header key") {
		t.Errorf("error %q does not mention unknown header", err)
	}
}

func TestParseErrorMissingTransitions(t *testing.T) {
	_, err := Parse("x.sm", []byte(`package: main`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "missing required block: transitions") {
		t.Errorf("error %q does not mention missing transitions", err)
	}
}

func TestParseErrorIncludesPosition(t *testing.T) {
	_, err := Parse("x.sm", []byte(`package: main
transitions: {
    *A + B = C,
    @bad@
}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "x.sm:") {
		t.Errorf("error %q does not start with filename:line:col", err)
	}
}
