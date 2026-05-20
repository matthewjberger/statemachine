# Usage

End-to-end guide for using `statemachine` in a Go project. Covers the CLI, the `go generate` workflow, the library API for tooling that doesn't want to shell out, and the wrapper patterns that connect generated code to your application's guards and side effects.

## Installation

```bash
go install github.com/matthewjberger/statemachine/cmd/statemachine@latest
```

This puts the `statemachine` binary in `$GOBIN` (or `$GOPATH/bin`). It must be on `PATH` for `go generate` directives to find it. Requires Go 1.22 or newer.

## CLI

```
statemachine [-out FILE] INPUT.sm
```

Flags:

- **`-out FILE`** — output path. Default: `INPUT_gen.go` in the same directory as `INPUT`.

Examples:

```bash
statemachine traffic.sm                       # writes traffic_gen.go
statemachine -out gen/state.go traffic.sm     # writes gen/state.go
```

The CLI reads the input, parses, validates, generates, runs the output through `go/format`, and writes the file atomically. Any error (parse, validation, or write) prints `file:line:column: message` to stderr and exits with status 1.

## go generate workflow

The intended way to drive the generator. Put a `//go:generate` directive in a Go source file alongside the `.sm`:

```
project/
├── state/
│   ├── state.sm
│   ├── state_gen.go        # generated; committed to VCS
│   └── doc.go              # holds the //go:generate directive
└── main.go
```

`state/doc.go`:

```go
//go:generate statemachine state.sm
package state
```

Then any of:

```bash
go generate ./state
go generate ./...
```

regenerates `state_gen.go` whenever `state.sm` changes. Commit the generated file alongside the source so consumers don't need to install the CLI just to build your project.

### Multiple state machines in one package

Drop a `//go:generate` line per `.sm`. Use the `name:` header in each `.sm` to namespace the generated identifiers:

```
state/
├── player.sm     // name: Player  → PlayerState, PlayerEvent, ...
├── enemy.sm      // name: Enemy   → EnemyState, EnemyEvent, ...
├── player_gen.go
├── enemy_gen.go
└── doc.go
```

```go
//go:generate statemachine player.sm
//go:generate statemachine enemy.sm
package state
```

## Library API

If you're writing tooling that needs to consume `.sm` files programmatically (a linter, a docs renderer, a different code generator), import the package directly:

```go
import "github.com/matthewjberger/statemachine"

source, err := os.ReadFile("traffic.sm")
if err != nil { ... }

ast, err := statemachine.Parse("traffic.sm", source)
if err != nil { ... }

if err := statemachine.Validate("traffic.sm", ast); err != nil { ... }

output, err := statemachine.Generate(ast)
if err != nil { ... }

// output is gofmt-clean Go source bytes
```

The AST types (`AST`, `Transition`, `Sources`, `SourceState`, `Target`, `Pos`) are exported, so you can walk the parsed structure without going through code generation. Useful for:

- Rendering the transition table as something other than Go (Mermaid, PlantUML, JSON for a debugger UI).
- Detecting unreachable states by walking the graph.
- Diffing two `.sm` files to flag breaking changes in CI.

The filename argument to `Parse` and `Validate` is used only for error messages. It does not need to refer to an actual file on disk.

## Connecting generated code to your application

The generator emits the pure transition table. Guards, side effects, error handling, and persistence are your wrapper's job. The conventional pattern in one block:

```go
type Robot struct {
    state         State
    position      int
    battery       int
}

func newRobot() *Robot {
    return &Robot{state: DefaultState(), battery: 100}
}

func (r *Robot) handle(event Event) error {
    // 1. Check if the transition is valid.
    next, ok := r.state.ProcessEvent(event)
    if !ok {
        return fmt.Errorf("event %s not valid from %s", event, r.state)
    }

    // 2. Apply guards. Reject before any side effect runs.
    if event == EventMove && r.battery < 5 {
        return fmt.Errorf("battery too low to move")
    }

    // 3. Run side effects. Past this point the transition is committed.
    switch event {
    case EventMove:
        r.battery -= 5
    case EventCharge:
        r.battery = 100
    }

    // 4. Commit the new state.
    r.state = next
    return nil
}
```

The shape is always the same: validate the transition first (cheap), then guards (medium), then side effects (potentially expensive), then commit. If a guard fails, you bail out before mutating any application state and the state machine is unchanged.

### Internal transitions

When the target is `_`, `next == r.state`. The wrapper code looks identical:

```go
case EventTick:
    r.movementTicks++   // side effect, no state change
```

The `r.state = next` line at the bottom does an assignment of the same value. No need to special-case.

### Guards that depend on the destination

Sometimes a guard depends on which state you're moving *to*, not the event itself:

```go
next, ok := r.state.ProcessEvent(event)
if !ok { return ... }

if next == StateMoving && r.battery < 5 {
    return errors.New("can't enter Moving with low battery")
}
```

This is the main reason `ProcessEvent` returns the destination instead of just `true`/`false`.

### Iterating valid events

`ValidEvents()` is useful for help text, autocomplete, AI controllers, or rejecting input early:

```go
fmt.Printf("Available actions from %s:\n", r.state)
for _, event := range r.state.ValidEvents() {
    fmt.Printf("  %s\n", event)
}
```

The slice is shared and reused on every call (see [CODEGEN.md](CODEGEN.md#validevents)), so don't mutate it.

### Persisting state

The state type is a `uint8`. Serialize it however you like — JSON, binary, a SQL `SMALLINT`. On the way back in, validate the integer is in range before assigning:

```go
var raw uint8
if err := db.QueryRow(...).Scan(&raw); err != nil { ... }
if raw >= uint8(len(AllStates)) {
    return errors.New("invalid persisted state")
}
state := State(raw)
```

`String()` is also a good persistence format if you'd rather decouple the on-disk representation from the integer values (which change if you reorder transitions). Parse back by linear scan over `AllStates`.

## Visualising the graph

Every generator output includes a `<Name>DOT` const with the Graphviz representation of the transition table. Render it:

```bash
go run ./cmd/print-dot | dot -Tpng -o states.png
```

```go
// cmd/print-dot/main.go
package main

import (
    "fmt"
    "your.module/state"
)

func main() { fmt.Println(state.LightDOT) }
```

Or, for ad-hoc inspection, paste the value into [GraphvizOnline](https://dreampuf.github.io/GraphvizOnline). The initial state is rendered as a `doublecircle`; wildcard transitions are expanded into explicit edges so the rendered graph is exhaustive.

## Errors

All errors from the library and the CLI follow `file:line:column: message`. This is parseable by every Go-aware editor's error pane and by `gnu errors` matchers in CI logs. The two error types are:

- `*statemachine.ParseError` — lexer or parser error.
- `*statemachine.ValidationError` — structural rule violation.

Both satisfy `error` and embed `Pos{Line, Column}` plus `File string`. If you need to react to a specific error programmatically, do a `errors.As` against the concrete type and inspect the `Message` field.
