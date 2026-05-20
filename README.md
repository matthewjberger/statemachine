# statemachine

A small state machine library for Go. Declare a transition table as plain data, validate once into an immutable `*Machine`. Guards and side effects live in your code.

## Install

```bash
go get github.com/matthewjberger/statemachine
```

Go 1.22+.

## Example

```go
package main

import (
    "fmt"

    "github.com/matthewjberger/statemachine"
)

type State uint8

const (
    Red State = iota
    Green
    Yellow
)

type Event uint8

const (
    Tick Event = iota
    Reset
)

var machine = statemachine.MustNew(statemachine.Spec[State, Event]{
    Initial: Red,
    Transitions: []statemachine.Transition[State, Event]{
        {From: Red, Event: Tick, To: Green},
        {From: Green, Event: Tick, To: Yellow},
        {From: Yellow, Event: Tick, To: Red},
    },
    Wildcards: []statemachine.Wildcard[State, Event]{
        {Event: Reset, To: Red},
    },
})

func main() {
    next, ok := machine.ProcessEvent(Red, Tick)
    fmt.Println(next, ok)
}
```

## Semantics

- Each `(From, Event)` pair and each wildcard event must be unique.
- Specific transitions win over wildcards on the same event.
- `ProcessEvent` returns `(next, true)` on a valid transition, `(state, false)` otherwise.
- `Machine` is immutable after construction; safe for concurrent reads.
- Slices returned by accessors alias internal storage; do not mutate.

See [`examples/`](examples) and [pkg.go.dev](https://pkg.go.dev/github.com/matthewjberger/statemachine) for the full API.

## License

MIT or Apache-2.0.
