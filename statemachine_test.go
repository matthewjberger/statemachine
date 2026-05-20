package statemachine_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/matthewjberger/statemachine"
)

type lightState uint8

const (
	red lightState = iota
	green
	yellow
)

func (s lightState) String() string {
	switch s {
	case red:
		return "Red"
	case green:
		return "Green"
	case yellow:
		return "Yellow"
	}
	return "Unknown"
}

type lightEvent uint8

const (
	tick lightEvent = iota
	reset
)

func (e lightEvent) String() string {
	switch e {
	case tick:
		return "Tick"
	case reset:
		return "Reset"
	}
	return "Unknown"
}

func newTrafficMachine(t *testing.T) *statemachine.Machine[lightState, lightEvent] {
	t.Helper()
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: tick, To: green},
			{From: green, Event: tick, To: yellow},
			{From: yellow, Event: tick, To: red},
		},
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return machine
}

func TestProcessEventSpecific(t *testing.T) {
	machine := newTrafficMachine(t)
	next, ok := machine.ProcessEvent(red, tick)
	if !ok || next != green {
		t.Fatalf("Red + Tick = %v ok=%v; want Green true", next, ok)
	}
}

func TestProcessEventWildcard(t *testing.T) {
	machine := newTrafficMachine(t)
	for _, from := range []lightState{red, green, yellow} {
		next, ok := machine.ProcessEvent(from, reset)
		if !ok || next != red {
			t.Fatalf("%v + Reset = %v ok=%v; want Red true", from, next, ok)
		}
	}
}

func TestProcessEventSpecificBeatsWildcard(t *testing.T) {
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: reset, To: yellow},
		},
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	next, _ := machine.ProcessEvent(red, reset)
	if next != yellow {
		t.Fatalf("Red + Reset = %v; want Yellow (specific wins over wildcard)", next)
	}
	next, _ = machine.ProcessEvent(green, reset)
	if next != red {
		t.Fatalf("Green + Reset = %v; want Red (wildcard fallback)", next)
	}
}

func TestProcessEventNoMatch(t *testing.T) {
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: tick, To: green},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	next, ok := machine.ProcessEvent(green, tick)
	if ok {
		t.Fatalf("Green + Tick = %v ok=true; want no match", next)
	}
	if next != green {
		t.Fatalf("no-match returned %v; want input state green", next)
	}
}

func TestValidEvents(t *testing.T) {
	machine := newTrafficMachine(t)
	got := machine.ValidEvents(red)
	want := []lightEvent{tick, reset}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ValidEvents(Red) = %v; want %v", got, want)
	}
}

func TestValidEventsDedupesWildcard(t *testing.T) {
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: reset, To: green},
		},
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := machine.ValidEvents(red)
	if len(got) != 1 || got[0] != reset {
		t.Fatalf("ValidEvents(Red) = %v; want [Reset] (deduped)", got)
	}
}

func TestStatesAndEventsDeclarationOrder(t *testing.T) {
	machine := newTrafficMachine(t)
	wantStates := []lightState{red, green, yellow}
	if !reflect.DeepEqual(machine.States(), wantStates) {
		t.Fatalf("States = %v; want %v", machine.States(), wantStates)
	}
	wantEvents := []lightEvent{tick, reset}
	if !reflect.DeepEqual(machine.Events(), wantEvents) {
		t.Fatalf("Events = %v; want %v", machine.Events(), wantEvents)
	}
}

func TestInitial(t *testing.T) {
	machine := newTrafficMachine(t)
	if machine.Initial() != red {
		t.Fatalf("Initial = %v; want Red", machine.Initial())
	}
}

func TestNewRejectsEmptySpec(t *testing.T) {
	_, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{Initial: red})
	if err == nil {
		t.Fatal("New on empty spec succeeded; want error")
	}
}

func TestNewRejectsDuplicateTransition(t *testing.T) {
	_, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: tick, To: green},
			{From: red, Event: tick, To: yellow},
		},
	})
	if err == nil {
		t.Fatal("New with duplicate (from, event) succeeded; want error")
	}
}

func TestNewRejectsDuplicateWildcard(t *testing.T) {
	_, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
			{Event: reset, To: green},
		},
	})
	if err == nil {
		t.Fatal("New with duplicate wildcard event succeeded; want error")
	}
}

func TestMustNewPanicsOnInvalidSpec(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("MustNew on invalid spec did not panic")
		}
	}()
	statemachine.MustNew(statemachine.Spec[lightState, lightEvent]{Initial: red})
}

func TestDOTContainsExpectedEdges(t *testing.T) {
	machine := newTrafficMachine(t)
	dot := machine.DOT()
	expected := []string{
		`"Red" [shape=doublecircle];`,
		`"Red" -> "Green" [label="Tick"];`,
		`"Green" -> "Yellow" [label="Tick"];`,
		`"Yellow" -> "Red" [label="Tick"];`,
		`"Red" -> "Red" [label="Reset"];`,
		`"Green" -> "Red" [label="Reset"];`,
		`"Yellow" -> "Red" [label="Reset"];`,
	}
	for _, line := range expected {
		if !strings.Contains(dot, line) {
			t.Errorf("DOT missing %q\n--- got ---\n%s", line, dot)
		}
	}
}

func BenchmarkProcessEventSpecific(b *testing.B) {
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: tick, To: green},
			{From: green, Event: tick, To: yellow},
			{From: yellow, Event: tick, To: red},
		},
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	state := red
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		next, _ := machine.ProcessEvent(state, tick)
		state = next
	}
}

func BenchmarkProcessEventWildcard(b *testing.B) {
	machine, err := statemachine.New(statemachine.Spec[lightState, lightEvent]{
		Initial: red,
		Transitions: []statemachine.Transition[lightState, lightEvent]{
			{From: red, Event: tick, To: green},
			{From: green, Event: tick, To: yellow},
			{From: yellow, Event: tick, To: red},
		},
		Wildcards: []statemachine.Wildcard[lightState, lightEvent]{
			{Event: reset, To: red},
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		machine.ProcessEvent(yellow, reset)
	}
}
