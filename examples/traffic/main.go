//go:generate statemachine traffic.sm
package main

import "fmt"

type Light struct {
	state LightState
	ticks int
}

func newLight() *Light {
	return &Light{state: DefaultLightState()}
}

func (l *Light) handle(event LightEvent) {
	next, ok := l.state.ProcessEvent(event)
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
	fmt.Printf("States: %v\n", AllLightStates)
	fmt.Printf("Events: %v\n", AllLightEvents)
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
	for _, state := range AllLightStates {
		fmt.Printf("  %s: %v\n", state, state.ValidEvents())
	}
}
