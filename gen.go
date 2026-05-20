package statemachine

import (
	"bytes"
	"fmt"
	"go/format"
	"strings"
)

// Generate produces the Go source for ast. The returned bytes have already been
// run through go/format. The caller is responsible for writing them out.
func Generate(ast *AST) ([]byte, error) {
	model := buildModel(ast)
	var buf bytes.Buffer
	writeFile(&buf, model)
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated source: %w\n\n%s", err, buf.String())
	}
	return formatted, nil
}

// model is the codegen-ready view of a parsed AST: names already resolved,
// states/events deduplicated in declaration order, transitions split into
// specific and wildcard buckets.
type model struct {
	Package      string
	StateType    string
	EventType    string
	StatePrefix  string
	EventPrefix  string
	DefaultFunc  string
	AllStatesVar string
	AllEventsVar string
	DOTConstName string

	States  []string
	Events  []string
	Initial string

	Specific []specificTransition
	Wildcard []wildcardTransition

	ValidEvents []validEventsEntry
	DOT         string
}

// specificTransition is one resolved (sourceState, event) -> destState mapping.
// Internal (`_`) targets are resolved to the source state.
type specificTransition struct {
	Source string
	Event  string
	Dest   string
}

// wildcardTransition is `_ + event = dest`. Internal target is represented by Dest == "".
type wildcardTransition struct {
	Event    string
	Dest     string
	Internal bool
}

// validEventsEntry is the events list for one state, in declaration order,
// including wildcard events that apply because the state lacks a specific transition for that event.
type validEventsEntry struct {
	State  string
	Events []string
}

func buildModel(ast *AST) *model {
	states := stateOrder(ast.Transitions)
	events := eventOrder(ast.Transitions)
	initial := initialState(ast)

	statePrefix := "State"
	eventPrefix := "Event"
	stateType := "State"
	eventType := "Event"
	defaultFunc := "DefaultState"
	allStatesVar := "AllStates"
	allEventsVar := "AllEvents"
	dotConst := "DOT"
	if ast.Name != "" {
		stateType = ast.Name + "State"
		eventType = ast.Name + "Event"
		statePrefix = stateType
		eventPrefix = eventType
		defaultFunc = "Default" + stateType
		allStatesVar = "All" + stateType + "s"
		allEventsVar = "All" + eventType + "s"
		dotConst = ast.Name + "DOT"
	}

	model := &model{
		Package:      ast.Package,
		StateType:    stateType,
		EventType:    eventType,
		StatePrefix:  statePrefix,
		EventPrefix:  eventPrefix,
		DefaultFunc:  defaultFunc,
		AllStatesVar: allStatesVar,
		AllEventsVar: allEventsVar,
		DOTConstName: dotConst,
		States:       states,
		Events:       events,
		Initial:      initial,
	}

	specificPairs := map[[2]string]bool{}
	for _, transition := range ast.Transitions {
		if transition.Sources.Wildcard {
			continue
		}
		for _, source := range transition.Sources.States {
			for _, event := range transition.Events {
				dest := source.Name
				if !transition.Target.Internal {
					dest = transition.Target.State
				}
				model.Specific = append(model.Specific, specificTransition{
					Source: source.Name,
					Event:  event,
					Dest:   dest,
				})
				specificPairs[[2]string{source.Name, event}] = true
			}
		}
	}

	for _, transition := range ast.Transitions {
		if !transition.Sources.Wildcard {
			continue
		}
		for _, event := range transition.Events {
			model.Wildcard = append(model.Wildcard, wildcardTransition{
				Event:    event,
				Dest:     transition.Target.State,
				Internal: transition.Target.Internal,
			})
		}
	}

	wildcardEventOrder := []string{}
	seenWildcardEvent := map[string]bool{}
	for _, transition := range ast.Transitions {
		if !transition.Sources.Wildcard {
			continue
		}
		for _, event := range transition.Events {
			if seenWildcardEvent[event] {
				continue
			}
			seenWildcardEvent[event] = true
			wildcardEventOrder = append(wildcardEventOrder, event)
		}
	}

	for _, state := range states {
		var entry validEventsEntry
		entry.State = state
		seen := map[string]bool{}
		for _, transition := range ast.Transitions {
			if transition.Sources.Wildcard {
				continue
			}
			containsState := false
			for _, source := range transition.Sources.States {
				if source.Name == state {
					containsState = true
					break
				}
			}
			if !containsState {
				continue
			}
			for _, event := range transition.Events {
				if seen[event] {
					continue
				}
				seen[event] = true
				entry.Events = append(entry.Events, event)
			}
		}
		for _, event := range wildcardEventOrder {
			if specificPairs[[2]string{state, event}] {
				continue
			}
			if seen[event] {
				continue
			}
			seen[event] = true
			entry.Events = append(entry.Events, event)
		}
		model.ValidEvents = append(model.ValidEvents, entry)
	}

	model.DOT = buildDOT(ast, states, specificPairs, initial)
	return model
}

func buildDOT(ast *AST, states []string, specificPairs map[[2]string]bool, initial string) string {
	var dot strings.Builder
	dot.WriteString("digraph {\n  rankdir=LR;\n  node [shape=circle];\n")
	if initial != "" {
		fmt.Fprintf(&dot, "  %q [shape=doublecircle];\n", initial)
	}
	for _, transition := range ast.Transitions {
		if transition.Sources.Wildcard {
			continue
		}
		for _, source := range transition.Sources.States {
			for _, event := range transition.Events {
				target := source.Name
				if !transition.Target.Internal {
					target = transition.Target.State
				}
				fmt.Fprintf(&dot, "  %q -> %q [label=%q];\n", source.Name, target, event)
			}
		}
	}
	for _, transition := range ast.Transitions {
		if !transition.Sources.Wildcard {
			continue
		}
		for _, state := range states {
			for _, event := range transition.Events {
				if specificPairs[[2]string{state, event}] {
					continue
				}
				target := state
				if !transition.Target.Internal {
					target = transition.Target.State
				}
				fmt.Fprintf(&dot, "  %q -> %q [label=%q];\n", state, target, event)
			}
		}
	}
	dot.WriteString("}")
	return dot.String()
}

func writeFile(buf *bytes.Buffer, model *model) {
	fmt.Fprintf(buf, "// Code generated by github.com/matthewjberger/statemachine; DO NOT EDIT.\n\n")
	fmt.Fprintf(buf, "package %s\n\n", model.Package)

	writeEnum(buf, model.StateType, model.StatePrefix, model.States)
	writeStringer(buf, "s", model.StateType, model.StatePrefix, model.States)
	writeAllVar(buf, model.AllStatesVar, model.StateType, model.StatePrefix, model.States)

	writeEnum(buf, model.EventType, model.EventPrefix, model.Events)
	writeStringer(buf, "e", model.EventType, model.EventPrefix, model.Events)
	writeAllVar(buf, model.AllEventsVar, model.EventType, model.EventPrefix, model.Events)

	writeDefault(buf, model)
	writeProcessEvent(buf, model)
	writeValidEventsTables(buf, model)
	writeValidEvents(buf, model)
	writeDOT(buf, model)
}

func writeEnum(buf *bytes.Buffer, typeName, prefix string, names []string) {
	fmt.Fprintf(buf, "type %s uint8\n\n", typeName)
	fmt.Fprintf(buf, "const (\n")
	for index, name := range names {
		if index == 0 {
			fmt.Fprintf(buf, "\t%s%s %s = iota\n", prefix, name, typeName)
			continue
		}
		fmt.Fprintf(buf, "\t%s%s\n", prefix, name)
	}
	fmt.Fprintf(buf, ")\n\n")
}

func writeStringer(buf *bytes.Buffer, receiver, typeName, prefix string, names []string) {
	fmt.Fprintf(buf, "func (%s %s) String() string {\n", receiver, typeName)
	fmt.Fprintf(buf, "\tswitch %s {\n", receiver)
	for _, name := range names {
		fmt.Fprintf(buf, "\tcase %s%s:\n\t\treturn %q\n", prefix, name, name)
	}
	fmt.Fprintf(buf, "\t}\n\treturn \"Unknown\"\n}\n\n")
}

func writeAllVar(buf *bytes.Buffer, varName, typeName, prefix string, names []string) {
	fmt.Fprintf(buf, "var %s = [...]%s{", varName, typeName)
	for index, name := range names {
		if index > 0 {
			fmt.Fprintf(buf, ", ")
		}
		fmt.Fprintf(buf, "%s%s", prefix, name)
	}
	fmt.Fprintf(buf, "}\n\n")
}

func writeDefault(buf *bytes.Buffer, model *model) {
	fmt.Fprintf(buf, "func %s() %s {\n", model.DefaultFunc, model.StateType)
	fmt.Fprintf(buf, "\treturn %s%s\n", model.StatePrefix, model.Initial)
	fmt.Fprintf(buf, "}\n\n")
}

func writeProcessEvent(buf *bytes.Buffer, model *model) {
	fmt.Fprintf(buf, "func (s %s) ProcessEvent(event %s) (%s, bool) {\n", model.StateType, model.EventType, model.StateType)

	bySource := map[string][]specificTransition{}
	sourceOrder := []string{}
	for _, transition := range model.Specific {
		if _, ok := bySource[transition.Source]; !ok {
			sourceOrder = append(sourceOrder, transition.Source)
		}
		bySource[transition.Source] = append(bySource[transition.Source], transition)
	}

	if len(model.Specific) > 0 {
		fmt.Fprintf(buf, "\tswitch s {\n")
		for _, source := range sourceOrder {
			fmt.Fprintf(buf, "\tcase %s%s:\n", model.StatePrefix, source)
			fmt.Fprintf(buf, "\t\tswitch event {\n")
			for _, transition := range bySource[source] {
				fmt.Fprintf(buf, "\t\tcase %s%s:\n", model.EventPrefix, transition.Event)
				fmt.Fprintf(buf, "\t\t\treturn %s%s, true\n", model.StatePrefix, transition.Dest)
			}
			fmt.Fprintf(buf, "\t\t}\n")
		}
		fmt.Fprintf(buf, "\t}\n")
	}

	if len(model.Wildcard) > 0 {
		fmt.Fprintf(buf, "\tswitch event {\n")
		for _, transition := range model.Wildcard {
			fmt.Fprintf(buf, "\tcase %s%s:\n", model.EventPrefix, transition.Event)
			if transition.Internal {
				fmt.Fprintf(buf, "\t\treturn s, true\n")
				continue
			}
			fmt.Fprintf(buf, "\t\treturn %s%s, true\n", model.StatePrefix, transition.Dest)
		}
		fmt.Fprintf(buf, "\t}\n")
	}

	fmt.Fprintf(buf, "\treturn s, false\n}\n\n")
}

func writeValidEventsTables(buf *bytes.Buffer, model *model) {
	if len(model.ValidEvents) == 0 {
		return
	}
	fmt.Fprintf(buf, "var (\n")
	for _, entry := range model.ValidEvents {
		fmt.Fprintf(buf, "\t%s = []%s{", validEventsVarName(model, entry.State), model.EventType)
		for index, event := range entry.Events {
			if index > 0 {
				fmt.Fprintf(buf, ", ")
			}
			fmt.Fprintf(buf, "%s%s", model.EventPrefix, event)
		}
		fmt.Fprintf(buf, "}\n")
	}
	fmt.Fprintf(buf, ")\n\n")
}

func writeValidEvents(buf *bytes.Buffer, model *model) {
	fmt.Fprintf(buf, "func (s %s) ValidEvents() []%s {\n", model.StateType, model.EventType)
	fmt.Fprintf(buf, "\tswitch s {\n")
	for _, entry := range model.ValidEvents {
		fmt.Fprintf(buf, "\tcase %s%s:\n", model.StatePrefix, entry.State)
		fmt.Fprintf(buf, "\t\treturn %s\n", validEventsVarName(model, entry.State))
	}
	fmt.Fprintf(buf, "\t}\n\treturn nil\n}\n\n")
}

func writeDOT(buf *bytes.Buffer, model *model) {
	fmt.Fprintf(buf, "const %s = `%s`\n", model.DOTConstName, model.DOT)
}

func validEventsVarName(model *model, state string) string {
	return lowerFirst(model.StatePrefix) + state + "ValidEvents"
}

func lowerFirst(value string) string {
	if value == "" {
		return value
	}
	return strings.ToLower(value[:1]) + value[1:]
}
