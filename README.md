# statemachine

A zero-cost state machine code generator for Go, port of [stateless](https://github.com/matthewjberger/stateless).

> **Related projects:**
> - [stateless](https://github.com/matthewjberger/stateless) - Rust original (proc-macro)

Most Go state machine libraries couple behavior to the state machine itself. `looplab/fsm` registers callbacks inside the constructor; `qmuntal/stateless` chains entry/exit/guards onto each transition via a fluent builder. The transition table and the code that runs alongside it are one object.

`statemachine` takes the opposite approach, ported from the Rust `stateless` crate: the `.sm` DSL is a pure transition table. The CLI generates two typed-int enums, a `ProcessEvent` method, and a few small helpers. Guards, side effects, and error handling live in your wrapper code, using normal Go patterns. The generated file doesn't know your types exist, and your types don't depend on any framework interface.

Where the Rust version is a `proc_macro` invoked at compile time, the Go version is a `go generate` codegen step. The runtime cost is the same: typed integers and switch statements, no maps, no reflection, no allocations on the hot path.

## Install

```bash
go install github.com/matthewjberger/statemachine/cmd/statemachine@latest
```

Requires Go 1.22+.

## Quick start

Write a `traffic.sm` file alongside your Go source:

```
package: traffic
name: Light

transitions: {
    *Red + Tick = Green,
    Green + Tick = Yellow,
    Yellow + Tick = Red,
    _ + Reset = Red,
}
```

Add a `go:generate` directive to a Go file in the same directory:

```go
//go:generate statemachine traffic.sm
package traffic
```

Run `go generate ./...`. The CLI writes `traffic_gen.go` next to the `.sm` source:

```go
type LightState uint8

const (
    LightStateRed LightState = iota
    LightStateGreen
    LightStateYellow
)

func DefaultLightState() LightState { return LightStateRed }

func (s LightState) ProcessEvent(event LightEvent) (LightState, bool) { ... }
func (s LightState) ValidEvents() []LightEvent                        { ... }
func (s LightState) String() string                                   { ... }

var AllLightStates = [...]LightState{...}
var AllLightEvents = [...]LightEvent{...}

const LightDOT = `digraph { ... }`
```

`ProcessEvent` returns `(newState, true)` if the transition is valid, `(currentState, false)` if not. This lets you insert guards and side effects between checking validity and applying the transition:

```go
type Light struct{ state LightState }

func (l *Light) tick() {
    next, ok := l.state.ProcessEvent(LightEventTick)
    if !ok {
        return
    }
    // guards and side effects here
    l.state = next
}
```

## DSL reference

```
package: traffic                         // Required: Go package for the generated file
name: Light                              // Optional: prefix for State, Event, Default*, All*s, DOT

transitions: {
    *Idle + Start = Running,             // Initial state marked with *
    Ready | Waiting + Start = Active,    // State patterns (multiple source states)
    Active + Stop | Pause = Idle,        // Event patterns (multiple trigger events)
    _ + Reset = Idle,                    // Wildcard source (lowest priority)
    Active + Tick = _,                   // Internal target (stay in same state)
}
```

`//` line comments are supported anywhere. Trailing commas are optional.

The CLI rejects invalid definitions:

- Empty transition blocks
- More than one state marked with `*`
- Duplicate `(state, event)` pairs
- Duplicate wildcard events

Errors point at `file:line:column`.

## Generated API

For `name: Light`, the generated identifiers carry the `Light` prefix; with no `name` directive the prefix is dropped.

| Name                              | Kind     | Purpose                                                            |
|-----------------------------------|----------|--------------------------------------------------------------------|
| `LightState`                      | type     | `uint8` enum of every state, in declaration order                  |
| `LightStateRed`, ...              | const    | one per state                                                      |
| `LightEvent`                      | type     | `uint8` enum of every event, in declaration order                  |
| `LightEventTick`, ...             | const    | one per event                                                      |
| `DefaultLightState()`             | func     | returns the `*`-marked state, or the first declared state          |
| `AllLightStates`                  | var      | `[N]LightState` array of every state                               |
| `AllLightEvents`                  | var      | `[N]LightEvent` array of every event                               |
| `(LightState).String()`           | method   | name of the state                                                  |
| `(LightEvent).String()`           | method   | name of the event                                                  |
| `(LightState).ProcessEvent(e)`    | method   | `(next, true)` for a valid transition, `(current, false)` if not   |
| `(LightState).ValidEvents()`      | method   | events that produce transitions from this state (wildcards merged) |
| `LightDOT`                        | const    | Graphviz DOT representation of the transition table                |

`ValidEvents` returns a package-scope slice per state, so it allocates nothing on the call.

`LightDOT` is a `const`. If you never reference it, it has zero binary footprint after the linker dead-codes it.

## Examples

- [`examples/traffic`](examples/traffic) - minimal `Light` machine: states, events, wildcards, ValidEvents
- [`examples/robot`](examples/robot) - port of the `stateless` Rust `demo.rs`, with guards, state patterns, internal transitions, and wildcards

```bash
just run-traffic
just run-robot
```

## Documentation

In-depth docs live under [`docs/`](docs):

- [Architecture](docs/ARCHITECTURE.md) - the lexer / parser / validator / codegen pipeline, file by file
- [DSL reference](docs/DSL.md) - the full `.sm` grammar, every header, every error
- [Generated code reference](docs/CODEGEN.md) - what gets emitted, why, and what it costs at runtime
- [Usage guide](docs/USAGE.md) - CLI flags, `go generate` integration, wrapper patterns
- [Porting from `stateless` (Rust)](docs/PORTING.md) - feature-by-feature mapping from the Rust crate
- [Releases](docs/RELEASES.md) - semver policy and tagging workflow

## Why a CLI and not a library

Go has no macro system, so the choice is between a runtime registration API (`fsm.New(...).On(...)`) and a code generator. A runtime API loses the compile-time validation, the zero-allocation `ValidEvents`, the `const` DOT output, and the typed-int enum (you'd need `map[State]map[Event]State` or similar). A code generator keeps all of those properties, at the cost of one `//go:generate` directive per machine.

The same trade-off the original Rust crate makes, in the equivalent Go shape.

## License

Dual-licensed under MIT and Apache-2.0. See [LICENSE-MIT](LICENSE-MIT) and [LICENSE-APACHE](LICENSE-APACHE). Pick whichever fits your project.
