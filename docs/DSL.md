# DSL Reference

The `.sm` source format is a small, statically-validated DSL that describes one state machine. This document is its full specification.

## File grammar

```
File             := Header* TransitionsBlock
Header           := "package" ":" Ident
                  | "name"    ":" Ident
TransitionsBlock := "transitions" ":" "{" TransitionList? "}"
TransitionList   := Transition ("," Transition)* ","?
Transition       := Sources "+" EventList "=" Target
Sources          := "_"
                  | ("*"? Ident) ("|" "*"? Ident)*
EventList        := Ident ("|" Ident)*
Target           := Ident
                  | "_"
Ident            := Letter (Letter | Digit | "_")*
Letter           := ASCII letter (A..Z, a..z) or "_"
Digit            := 0..9
Comment          := "//" until end of line
```

The lexer scans byte-by-byte, so non-ASCII identifiers (`é`, `ñ`, ...) are not supported even though Go accepts them. Stick to ASCII.

Whitespace (`\s`, `\t`, `\r`, `\n`) and `//` line comments may appear anywhere outside an identifier. Transitions inside the block are separated by `,`; the trailing comma after the last transition is optional but the separators between transitions are required. The order of headers is free; both `package: x\nname: Y` and `name: Y\npackage: x` parse identically.

## Headers

### `package:` (required)

Sets the Go package name of the generated file. Must be a valid Go identifier. If you generate alongside a Go source file with `package foo`, the `.sm` must also say `package: foo` (the CLI does not infer this — it does what the file says).

```
package: traffic
```

### `name:` (optional)

Adds a name prefix to every generated identifier:

| `name:` value     | State type     | Const example   | Default fn            | All-states var      | DOT const   |
|-------------------|----------------|-----------------|-----------------------|---------------------|-------------|
| (omitted)         | `State`        | `StateIdle`     | `DefaultState()`      | `AllStates`         | `DOT`       |
| `Light`           | `LightState`   | `LightStateRed` | `DefaultLightState()` | `AllLightStates`    | `LightDOT`  |
| `Player`          | `PlayerState`  | `PlayerStateIdle` | `DefaultPlayerState()` | `AllPlayerStates` | `PlayerDOT` |

Use `name:` when you have more than one state machine in the same Go package, so the generated identifiers don't collide.

## Transitions block

The `transitions:` block is required and must contain at least one transition. Each transition has three parts separated by `+` and `=`:

```
Sources + EventList = Target
```

### Sources

A `Sources` clause says **which states** can fire this transition. It takes one of two forms:

#### Named states

One or more state identifiers separated by `|`. Any state in the list can fire the transition:

```
Ready | Waiting + Start = Active
```

The first occurrence of any state across the whole file determines its position in `AllStates` (declaration order). The same state can appear in multiple transitions on either side of `=` without restriction.

#### Initial-state marker

Exactly one state in the file may be marked with `*`. That state becomes the value returned by `Default<Name>State()`. The marker goes immediately before the identifier:

```
*Idle + Start = Running
Ready | *Waiting + Start = Active   // legal: marker on second state in pattern
```

If no state is marked, the first state in declaration order is the default. Marking two states is a validation error (`multiple initial states: only one state can be marked with '*'`).

#### Wildcard source `_`

A bare `_` as the sources clause means "from any state":

```
_ + Reset = Idle
```

A wildcard transition cannot be combined with named states (`*Idle | _` is rejected at parse time). Wildcards have **lower priority** than specific transitions: if both `Idle + Reset = X` and `_ + Reset = Y` exist, `Idle.ProcessEvent(Reset)` returns `X`. This applies even when the wildcard transition is declared first in the file — priority is determined by structure, not source order.

The same event may not be the trigger of two wildcard transitions (`duplicate wildcard transition: '_ + Reset' is already defined`).

### EventList

One or more event identifiers separated by `|`. Any event in the list triggers the transition:

```
Active + Pause | Stop = Idle
```

Events do not need to be unique across the file; an event may appear in multiple transitions. The first occurrence determines its position in `AllEvents`.

### Target

One of:

- **A named state**: the destination after the transition fires.
- **`_`** (internal): the state does not change. The handler still returns `(currentState, true)` from `ProcessEvent`, signalling that the event was accepted, so wrapper code can run side effects without a state change.

```
Moving + Tick = _        // tick advances some counter, stays Moving
Moving + Arrive = Idle   // arrive transitions to Idle
```

## Comments

`//` starts a line comment that runs to the next newline. There is no block-comment form.

```
// header comment
package: main // trailing comment

transitions: {
    // a transition with a comment
    *Idle + Start = Running, // and one trailing the line
}
```

## Identifier rules

Identifiers start with an ASCII letter or `_` and continue with letters, digits, or `_`. The bare `_` is reserved (wildcard source or internal target) and cannot be used as a state, event, or header value. Identifiers like `_foo`, `Foo_Bar`, `state2` are all legal.

All identifiers in the DSL become Go identifiers in the generated code without transformation, so they must also be valid Go identifiers. The lexer scans byte-by-byte, so multi-byte UTF-8 letters do not work even though Go itself accepts them. Stick to ASCII.

## Compile-time validations

Errors are reported in `file:line:column: message` format and short-circuit code generation.

### Parse-time

| Trigger                                | Message                                          |
|----------------------------------------|--------------------------------------------------|
| Missing `package:` header              | `missing required header: package`               |
| Missing `transitions:` block           | `missing required block: transitions`            |
| Unknown header key                     | `unknown header key "X" (expected 'package', 'name', or 'transitions')` |
| Unexpected character                   | `unexpected character 'X'`                       |
| Wrong token at expected position       | `expected <kind>, got <kind>`                    |
| `_` mixed with named states in sources | `'_' wildcard cannot be combined with named states` |
| `_` used as an event                   | `events cannot be '_'`                           |

### Validation

| Trigger                                | Message                                          |
|----------------------------------------|--------------------------------------------------|
| Empty `transitions` block              | `state machine must have at least one transition` |
| Two `*` markers                        | `multiple initial states: only one state can be marked with '*'` |
| Same `(state, event)` pair twice       | `duplicate transition: state 'X' + event 'Y' is already defined` |
| Same wildcard event twice              | `duplicate wildcard transition: '_ + Y' is already defined` |

## Reference example

Every feature in one file:

```
// players have many states; lights have fewer.
// here's a player.
package: game
name: Player

transitions: {
    // start in Idle
    *Idle + Move = Walking,

    // running and walking are both Moving for tick purposes
    Walking | Running + Stop = Idle,

    // event patterns: two ways to enter Jumping
    Walking + Jump | DoubleTap = Jumping,

    // internal transition: tick a stamina counter without changing state
    Walking + Tick = _,

    // wildcard: damage from any state takes you to Hurt
    _ + Damage = Hurt,

    // wildcard with internal target: Heartbeat happens everywhere but doesn't change state
    _ + Heartbeat = _,
}
```

`AllPlayerStates` will be `[Idle Walking Running Jumping Hurt]` (declaration order, deduplicated). `DefaultPlayerState()` returns `PlayerStateIdle`. `PlayerStateWalking.ProcessEvent(PlayerEventDamage)` returns `(PlayerStateHurt, true)` (specific transitions for Walking don't include Damage, so the wildcard wins).
