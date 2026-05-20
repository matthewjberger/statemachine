package statemachine

import "fmt"

// ValidationError is returned by Validate when an AST violates one of the
// structural rules. It formats as `file:line:col: message`.
type ValidationError struct {
	File    string
	Pos     Pos
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Pos.Line, e.Pos.Column, e.Message)
}

// Validate enforces the same compile-time rules as the Rust stateless macro:
//   - at least one transition
//   - at most one state marked with '*'
//   - no duplicate (state, event) transitions
//   - no duplicate wildcard events
//
// The filename is only used in error messages.
func Validate(filename string, ast *AST) error {
	if len(ast.Transitions) == 0 {
		return &ValidationError{File: filename, Pos: Pos{Line: 1, Column: 1}, Message: "state machine must have at least one transition"}
	}

	initialCount := 0
	var initialPositions []Pos
	for _, transition := range ast.Transitions {
		for _, source := range transition.Sources.States {
			if source.Initial {
				initialCount++
				initialPositions = append(initialPositions, source.Pos)
			}
		}
	}
	if initialCount > 1 {
		return &ValidationError{File: filename, Pos: initialPositions[1], Message: "multiple initial states: only one state can be marked with '*'"}
	}

	type pair struct {
		state, event string
	}
	seenPair := map[pair]Pos{}
	seenWildcardEvent := map[string]Pos{}

	for _, transition := range ast.Transitions {
		if transition.Sources.Wildcard {
			for _, event := range transition.Events {
				if _, ok := seenWildcardEvent[event]; ok {
					return &ValidationError{File: filename, Pos: transition.Pos, Message: fmt.Sprintf("duplicate wildcard transition: '_ + %s' is already defined", event)}
				}
				seenWildcardEvent[event] = transition.Pos
			}
			continue
		}
		for _, source := range transition.Sources.States {
			for _, event := range transition.Events {
				key := pair{state: source.Name, event: event}
				if _, ok := seenPair[key]; ok {
					return &ValidationError{File: filename, Pos: transition.Pos, Message: fmt.Sprintf("duplicate transition: state '%s' + event '%s' is already defined", source.Name, event)}
				}
				seenPair[key] = transition.Pos
			}
		}
	}

	return nil
}
