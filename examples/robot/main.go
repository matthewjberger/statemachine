package main

import (
	"fmt"

	"github.com/matthewjberger/statemachine"
)

type State uint8

const (
	StateOff State = iota
	StateIdle
	StateMoving
	StateWaiting
	StateEmergencyStopped
)

func (s State) String() string {
	switch s {
	case StateOff:
		return "Off"
	case StateIdle:
		return "Idle"
	case StateMoving:
		return "Moving"
	case StateWaiting:
		return "Waiting"
	case StateEmergencyStopped:
		return "EmergencyStopped"
	}
	return "Unknown"
}

type Event uint8

const (
	EventPowerOn Event = iota
	EventMoveTo
	EventTick
	EventArrive
	EventObstacleDetected
	EventObstacleClear
	EventEmergencyStop
	EventReset
	EventPowerOff
)

func (e Event) String() string {
	switch e {
	case EventPowerOn:
		return "PowerOn"
	case EventMoveTo:
		return "MoveTo"
	case EventTick:
		return "Tick"
	case EventArrive:
		return "Arrive"
	case EventObstacleDetected:
		return "ObstacleDetected"
	case EventObstacleClear:
		return "ObstacleClear"
	case EventEmergencyStop:
		return "EmergencyStop"
	case EventReset:
		return "Reset"
	case EventPowerOff:
		return "PowerOff"
	}
	return "Unknown"
}

var robotMachine = statemachine.MustNew(statemachine.Spec[State, Event]{
	Initial: StateOff,
	Transitions: []statemachine.Transition[State, Event]{
		{From: StateOff, Event: EventPowerOn, To: StateIdle},
		{From: StateIdle, Event: EventMoveTo, To: StateMoving},
		{From: StateMoving, Event: EventTick, To: StateMoving},
		{From: StateMoving, Event: EventArrive, To: StateIdle},
		{From: StateMoving, Event: EventObstacleDetected, To: StateWaiting},
		{From: StateWaiting, Event: EventObstacleClear, To: StateMoving},
		{From: StateIdle, Event: EventEmergencyStop, To: StateEmergencyStopped},
		{From: StateMoving, Event: EventEmergencyStop, To: StateEmergencyStopped},
		{From: StateWaiting, Event: EventEmergencyStop, To: StateEmergencyStopped},
		{From: StateEmergencyStopped, Event: EventReset, To: StateIdle},
	},
	Wildcards: []statemachine.Wildcard[State, Event]{
		{Event: EventPowerOff, To: StateOff},
	},
})

type Robot struct {
	state         State
	position      int
	target        int
	hasTarget     bool
	obstacles     int
	battery       int
	movementTicks int
}

func newRobot() *Robot {
	return &Robot{state: robotMachine.Initial(), battery: 100}
}

func (r *Robot) powerOn() {
	next, ok := robotMachine.ProcessEvent(r.state, EventPowerOn)
	if !ok {
		return
	}
	fmt.Printf("  [Action] Engaging motors (battery: %d%%)\n", r.battery)
	r.state = next
}

func (r *Robot) powerOff() {
	next, ok := robotMachine.ProcessEvent(r.state, EventPowerOff)
	if !ok {
		return
	}
	r.position = 0
	r.hasTarget = false
	r.state = next
	fmt.Printf("  [State] Robot powered off\n")
}

func (r *Robot) moveTo(position int) {
	next, ok := robotMachine.ProcessEvent(r.state, EventMoveTo)
	if !ok {
		return
	}
	r.target = position
	r.hasTarget = true
	r.movementTicks = 0
	fmt.Printf("  [Action] Moving to position %d from %d\n", position, r.position)
	r.state = next
}

func (r *Robot) tick() {
	next, ok := robotMachine.ProcessEvent(r.state, EventTick)
	if !ok {
		return
	}
	r.movementTicks++
	fmt.Printf("  [Internal] Movement tick %d (still %s)\n", r.movementTicks, r.state)
	r.state = next
}

func (r *Robot) checkPosition() {
	if !r.hasTarget {
		return
	}
	if r.position != r.target {
		fmt.Printf("  [Info] Target not reached yet\n")
		return
	}
	next, ok := robotMachine.ProcessEvent(r.state, EventArrive)
	if !ok {
		return
	}
	fmt.Printf("  [Info] Position reached: %d\n", r.position)
	r.hasTarget = false
	r.state = next
}

func (r *Robot) obstacleDetected() {
	next, ok := robotMachine.ProcessEvent(r.state, EventObstacleDetected)
	if !ok {
		return
	}
	r.obstacles++
	fmt.Printf("  [State] Obstacle detected, waiting (count: %d)\n", r.obstacles)
	r.state = next
}

func (r *Robot) tryClearObstacle() {
	next, ok := robotMachine.ProcessEvent(r.state, EventObstacleClear)
	if !ok {
		return
	}
	if r.obstacles >= 3 {
		fmt.Printf("  [Guard] Too many obstacles, cannot continue\n")
		return
	}
	fmt.Printf("  [State] Resuming movement\n")
	r.state = next
}

func (r *Robot) emergencyStop() {
	next, ok := robotMachine.ProcessEvent(r.state, EventEmergencyStop)
	if !ok {
		return
	}
	fmt.Printf("  [State] EMERGENCY STOP ACTIVATED\n")
	r.state = next
}

func (r *Robot) tryReset() {
	next, ok := robotMachine.ProcessEvent(r.state, EventReset)
	if !ok {
		return
	}
	if r.battery <= 10 {
		fmt.Printf("  [Guard] Insufficient power to reset\n")
		return
	}
	r.state = next
}

func main() {
	robot := newRobot()

	fmt.Println("=== State Machine Info ===")
	fmt.Printf("States: %v\n", robotMachine.States())
	fmt.Printf("Events: %v\n\n", robotMachine.Events())

	fmt.Println("=== Valid Events Per State ===")
	for _, state := range robotMachine.States() {
		events := robotMachine.ValidEvents(state)
		if len(events) == 0 {
			fmt.Printf("  %s: terminal\n", state)
			continue
		}
		fmt.Printf("  %s: %v\n", state, events)
	}
	fmt.Println()

	fmt.Println("=== Startup ===")
	fmt.Printf("State: %s\n", robot.state)
	robot.powerOn()
	fmt.Println()

	fmt.Println("=== Normal Operation ===")
	robot.moveTo(100)
	robot.tick()
	robot.tick()
	robot.tick()
	robot.position = 100
	robot.checkPosition()
	fmt.Println()

	fmt.Println("=== Obstacle Handling ===")
	robot.moveTo(200)
	robot.obstacleDetected()
	robot.tryClearObstacle()
	robot.position = 200
	robot.checkPosition()
	fmt.Println()

	fmt.Println("=== Emergency Stop ===")
	robot.moveTo(300)
	robot.emergencyStop()
	robot.tryReset()
	fmt.Println()

	fmt.Println("=== Wildcard (from any state) ===")
	robot.powerOff()
	fmt.Println()

	fmt.Printf("Final position: %d\n", robot.position)
	fmt.Printf("Obstacles encountered: %d\n\n", robot.obstacles)

	fmt.Println("=== DOT ===")
	fmt.Println(robotMachine.DOT())
}
