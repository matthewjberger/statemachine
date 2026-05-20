//go:generate statemachine robot.sm
package main

import "fmt"

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
	return &Robot{state: DefaultState(), battery: 100}
}

func (r *Robot) powerOn() {
	next, ok := r.state.ProcessEvent(EventPowerOn)
	if !ok {
		return
	}
	fmt.Printf("  [Action] Engaging motors (battery: %d%%)\n", r.battery)
	r.state = next
}

func (r *Robot) powerOff() {
	next, ok := r.state.ProcessEvent(EventPowerOff)
	if !ok {
		return
	}
	r.position = 0
	r.hasTarget = false
	r.state = next
	fmt.Printf("  [State] Robot powered off\n")
}

func (r *Robot) moveTo(position int) {
	next, ok := r.state.ProcessEvent(EventMoveTo)
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
	next, ok := r.state.ProcessEvent(EventTick)
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
	next, ok := r.state.ProcessEvent(EventArrive)
	if !ok {
		return
	}
	fmt.Printf("  [Info] Position reached: %d\n", r.position)
	r.hasTarget = false
	r.state = next
}

func (r *Robot) obstacleDetected() {
	next, ok := r.state.ProcessEvent(EventObstacleDetected)
	if !ok {
		return
	}
	r.obstacles++
	fmt.Printf("  [State] Obstacle detected, waiting (count: %d)\n", r.obstacles)
	r.state = next
}

func (r *Robot) tryClearObstacle() {
	next, ok := r.state.ProcessEvent(EventObstacleClear)
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
	next, ok := r.state.ProcessEvent(EventEmergencyStop)
	if !ok {
		return
	}
	fmt.Printf("  [State] EMERGENCY STOP ACTIVATED\n")
	r.state = next
}

func (r *Robot) tryReset() {
	next, ok := r.state.ProcessEvent(EventReset)
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
	fmt.Printf("States: %v\n", AllStates)
	fmt.Printf("Events: %v\n\n", AllEvents)

	fmt.Println("=== Valid Events Per State ===")
	for _, state := range AllStates {
		events := state.ValidEvents()
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
	fmt.Println(DOT)
}
