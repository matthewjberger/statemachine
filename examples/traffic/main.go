package main

import (
	"fmt"

	"github.com/matthewjberger/statemachine"
)

type LightState uint8

const (
	LightStateRed LightState = iota
	LightStateGreen
	LightStateYellow
)

func (s LightState) String() string {
	switch s {
	case LightStateRed:
		return "Red"
	case LightStateGreen:
		return "Green"
	case LightStateYellow:
		return "Yellow"
	}
	return "Unknown"
}

type LightEvent uint8

const (
	LightEventTick LightEvent = iota
	LightEventReset
)

func (e LightEvent) String() string {
	switch e {
	case LightEventTick:
		return "Tick"
	case LightEventReset:
		return "Reset"
	}
	return "Unknown"
}

var lightMachine = statemachine.MustNew(statemachine.Spec[LightState, LightEvent]{
	Initial: LightStateRed,
	Transitions: []statemachine.Transition[LightState, LightEvent]{
		{From: LightStateRed, Event: LightEventTick, To: LightStateGreen},
		{From: LightStateGreen, Event: LightEventTick, To: LightStateYellow},
		{From: LightStateYellow, Event: LightEventTick, To: LightStateRed},
	},
	Wildcards: []statemachine.Wildcard[LightState, LightEvent]{
		{Event: LightEventReset, To: LightStateRed},
	},
})

type Light struct {
	state LightState
	ticks int
}

func newLight() *Light {
	return &Light{state: lightMachine.Initial()}
}

func (l *Light) handle(event LightEvent) {
	next, ok := lightMachine.ProcessEvent(l.state, event)
	if !ok {
		fmt.Printf("  %s + %s: ignored (no transition)\n", l.state, event)
		return
	}
	if event == LightEventTick {
		l.ticks++
	}
	fmt.Printf("  %s + %s -> %s\n", l.state, event, next)
	l.state = next
}

func main() {
	light := newLight()
	fmt.Printf("States: %v\n", lightMachine.States())
	fmt.Printf("Events: %v\n", lightMachine.Events())
	fmt.Printf("Initial: %s\n\n", light.state)

	for _, event := range []LightEvent{
		LightEventTick,
		LightEventTick,
		LightEventTick,
		LightEventTick,
		LightEventReset,
	} {
		light.handle(event)
	}

	fmt.Printf("\nTotal ticks: %d\n", light.ticks)
	fmt.Printf("\nValidEvents per state:\n")
	for _, state := range lightMachine.States() {
		fmt.Printf("  %s: %v\n", state, lightMachine.ValidEvents(state))
	}
}
