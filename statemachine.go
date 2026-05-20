// Package statemachine is a small, allocation-light state machine library.
//
// A machine is declared once as a pure data Spec, validated, and turned into
// an immutable Machine whose transition table is laid out as packed slices
// scanned linearly at dispatch time. There are no maps on the hot path, no
// reflection, no allocations on ProcessEvent.
package statemachine

import (
	"errors"
	"fmt"
	"strings"
)

// Transition is a single (from + event -> to) entry in the table.
type Transition[S, E comparable] struct {
	From  S
	Event E
	To    S
}

// Wildcard matches an event from any state. It is consulted only when no
// specific Transition matches, so specific transitions always win.
type Wildcard[S, E comparable] struct {
	Event E
	To    S
}

// Spec is the pure-data declaration of a state machine.
//
// Initial is required. Either Transitions or Wildcards must be non-empty.
// The declaration order of states and events (first appearance across
// Initial, Transitions, Wildcards) is preserved in Machine.States and
// Machine.Events.
type Spec[S, E comparable] struct {
	Initial     S
	Transitions []Transition[S, E]
	Wildcards   []Wildcard[S, E]
}

// Machine is an immutable, validated state machine.
//
// All fields are read-only after construction. Methods are safe to call
// concurrently from multiple goroutines.
type Machine[S, E comparable] struct {
	initial      S
	states       []S
	events       []E
	transitions  []Transition[S, E]
	wildcards    []Wildcard[S, E]
	validByState [][]E
}

// New builds a Machine from the given Spec. It returns an error if the spec
// is invalid: no initial state set, no transitions declared, a duplicate
// (from, event) pair, or a duplicate wildcard event.
func New[S, E comparable](spec Spec[S, E]) (*Machine[S, E], error) {
	if len(spec.Transitions) == 0 && len(spec.Wildcards) == 0 {
		return nil, errors.New("statemachine: spec declares no transitions")
	}

	seenTransitions := make(map[transitionKey[S, E]]struct{}, len(spec.Transitions))
	for _, transition := range spec.Transitions {
		key := transitionKey[S, E]{From: transition.From, Event: transition.Event}
		if _, dup := seenTransitions[key]; dup {
			return nil, fmt.Errorf("statemachine: duplicate transition for (%v, %v)", transition.From, transition.Event)
		}
		seenTransitions[key] = struct{}{}
	}

	seenWildcards := make(map[E]struct{}, len(spec.Wildcards))
	for _, wildcard := range spec.Wildcards {
		if _, dup := seenWildcards[wildcard.Event]; dup {
			return nil, fmt.Errorf("statemachine: duplicate wildcard for event %v", wildcard.Event)
		}
		seenWildcards[wildcard.Event] = struct{}{}
	}

	stateIndex := make(map[S]int)
	var states []S
	recordState := func(state S) {
		if _, ok := stateIndex[state]; ok {
			return
		}
		stateIndex[state] = len(states)
		states = append(states, state)
	}

	eventIndex := make(map[E]struct{})
	var events []E
	recordEvent := func(event E) {
		if _, ok := eventIndex[event]; ok {
			return
		}
		eventIndex[event] = struct{}{}
		events = append(events, event)
	}

	recordState(spec.Initial)
	for _, transition := range spec.Transitions {
		recordState(transition.From)
		recordState(transition.To)
		recordEvent(transition.Event)
	}
	for _, wildcard := range spec.Wildcards {
		recordState(wildcard.To)
		recordEvent(wildcard.Event)
	}

	transitions := append([]Transition[S, E](nil), spec.Transitions...)
	wildcards := append([]Wildcard[S, E](nil), spec.Wildcards...)

	validByState := make([][]E, len(states))
	for index, state := range states {
		seen := make(map[E]struct{})
		var valid []E
		for _, transition := range transitions {
			if transition.From != state {
				continue
			}
			if _, ok := seen[transition.Event]; ok {
				continue
			}
			seen[transition.Event] = struct{}{}
			valid = append(valid, transition.Event)
		}
		for _, wildcard := range wildcards {
			if _, ok := seen[wildcard.Event]; ok {
				continue
			}
			seen[wildcard.Event] = struct{}{}
			valid = append(valid, wildcard.Event)
		}
		validByState[index] = valid
	}

	return &Machine[S, E]{
		initial:      spec.Initial,
		states:       states,
		events:       events,
		transitions:  transitions,
		wildcards:    wildcards,
		validByState: validByState,
	}, nil
}

// MustNew is like New but panics on validation error. Intended for
// package-level vars where the spec is a compile-time constant.
func MustNew[S, E comparable](spec Spec[S, E]) *Machine[S, E] {
	machine, err := New(spec)
	if err != nil {
		panic(err)
	}
	return machine
}

type transitionKey[S, E comparable] struct {
	From  S
	Event E
}

// Initial returns the machine's initial state.
func (m *Machine[S, E]) Initial() S { return m.initial }

// States returns every state in declaration order. The returned slice
// aliases internal storage; do not mutate it.
func (m *Machine[S, E]) States() []S { return m.states }

// Events returns every event in declaration order. The returned slice
// aliases internal storage; do not mutate it.
func (m *Machine[S, E]) Events() []E { return m.events }

// Transitions returns every specific transition in declaration order.
// The returned slice aliases internal storage; do not mutate it.
func (m *Machine[S, E]) Transitions() []Transition[S, E] { return m.transitions }

// Wildcards returns every wildcard in declaration order. The returned
// slice aliases internal storage; do not mutate it.
func (m *Machine[S, E]) Wildcards() []Wildcard[S, E] { return m.wildcards }

// ProcessEvent returns (next, true) if event triggers a transition from
// state, otherwise (state, false). Specific transitions are scanned before
// wildcards, so a specific (from, event) entry always wins over a wildcard
// on the same event.
//
// Dispatch is a linear scan of two packed slices. No allocations.
func (m *Machine[S, E]) ProcessEvent(state S, event E) (S, bool) {
	for index := range m.transitions {
		transition := &m.transitions[index]
		if transition.From == state && transition.Event == event {
			return transition.To, true
		}
	}
	for index := range m.wildcards {
		wildcard := &m.wildcards[index]
		if wildcard.Event == event {
			return wildcard.To, true
		}
	}
	return state, false
}

// ValidEvents returns the events that trigger a transition from state.
// Wildcards are merged in, deduplicated against specific events.
//
// The returned slice is a package-internal precomputed slice; it allocates
// nothing on the call but should not be mutated.
func (m *Machine[S, E]) ValidEvents(state S) []E {
	for index, candidate := range m.states {
		if candidate == state {
			return m.validByState[index]
		}
	}
	return nil
}

// DOT returns a Graphviz DOT representation of the machine. State and event
// names come from fmt.Sprint, so a custom String() method on S or E will be
// honored.
func (m *Machine[S, E]) DOT() string {
	var builder strings.Builder
	builder.WriteString("digraph {\n")
	builder.WriteString("  rankdir=LR;\n")
	builder.WriteString("  node [shape=circle];\n")
	fmt.Fprintf(&builder, "  %q [shape=doublecircle];\n", fmt.Sprint(m.initial))
	for _, transition := range m.transitions {
		fmt.Fprintf(&builder, "  %q -> %q [label=%q];\n",
			fmt.Sprint(transition.From),
			fmt.Sprint(transition.To),
			fmt.Sprint(transition.Event),
		)
	}
	for _, wildcard := range m.wildcards {
		for _, state := range m.states {
			fmt.Fprintf(&builder, "  %q -> %q [label=%q];\n",
				fmt.Sprint(state),
				fmt.Sprint(wildcard.To),
				fmt.Sprint(wildcard.Event),
			)
		}
	}
	builder.WriteString("}")
	return builder.String()
}
