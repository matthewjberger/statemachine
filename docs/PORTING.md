# Porting from `stateless` (Rust)

`statemachine` is a port of the Rust [`stateless`](https://github.com/matthewjberger/stateless) crate. This document maps every feature in the Rust crate to its Go equivalent, calls out what changed, and explains why.

## High-level shape

| Concern                  | Rust `stateless`                            | Go `statemachine`                             |
|--------------------------|---------------------------------------------|-----------------------------------------------|
| When the code is built   | Compile time (proc-macro expansion)         | `go generate` time (CLI invocation)           |
| Where the DSL lives      | Inline `statemachine! { ... }` block        | External `.sm` file                           |
| What it produces         | Tokens injected into the calling module     | A standalone `*_gen.go` file in the same package |
| Runtime cost             | Zero — switches on integer enums            | Zero — switches on `uint8` enums              |
| Validation timing        | Compile error (the macro `compile_error!`s) | CLI exit with `file:line:column: msg`         |

The structural promise — *transition table as data, behavior in your wrapper* — is identical.

## DSL syntax

The Go DSL is a near-literal port of the Rust syntax. Compare:

```rust
statemachine! {
    name: Player,
    derive_states: [Debug, Clone],
    transitions: {
        *Idle + Move = Walking,
        Walking + Stop = Idle,
        _ + Reset = Idle,
        Walking + Tick = _,
    }
}
```

```
package: game
name: Player

transitions: {
    *Idle + Move = Walking,
    Walking + Stop = Idle,
    _ + Reset = Idle,
    Walking + Tick = _,
}
```

The only differences:

- `package:` is **required** in Go (proc-macros don't need it because they inherit the calling module; CLIs do).
- The DSL is in a separate file because Go has no proc-macro system. The `package:` header lets the CLI emit the file into the right namespace.

## Generated API mapping

| Rust                            | Go                                                  | Notes |
|---------------------------------|-----------------------------------------------------|-------|
| `enum State { ... }`            | `type State uint8` + `const (...)`                  | Go has no real enums; typed-int constants are the idiom |
| `State::Idle`                   | `StateIdle`                                         | No `::` in Go; the prefix is folded into the name |
| `enum Event { ... }`            | `type Event uint8` + `const (...)`                  | Same |
| `State::default()`              | `DefaultState()`                                    | Go has no `Default` trait; explicit factory function |
| `state.process_event(event)`    | `state.ProcessEvent(event)`                         | Same shape; `Option<State>` → `(State, bool)`     |
| `Option::Some(s)` / `None`      | `(s, true)` / `(s, false)`                          | Go's idiom for "value or absence"                 |
| `State::ALL`                    | `AllStates` (a `[N]State` array)                    | No associated consts in Go; package-scope var     |
| `Event::ALL`                    | `AllEvents`                                         | Same                                              |
| `state.valid_events()`          | `state.ValidEvents() []Event`                       | Returns a slice over a package-scope backing array |
| `State::DOT`                    | `DOT` (or `<Name>DOT`)                              | `const string`, same dead-code semantics          |
| `derive_states: [...]`          | (no equivalent)                                     | See "Intentionally dropped" below                 |
| `derive_events: [...]`          | (no equivalent)                                     | See "Intentionally dropped" below                 |

The `name:` directive works identically: it adds a prefix to every generated identifier.

## Semantics that carry over unchanged

- **Specific-over-wildcard priority.** `*A + Reset = B, _ + Reset = Z` makes `A.ProcessEvent(Reset)` return `B`. The wildcard applies only to states without a specific entry for that event. Declaration order does not affect priority.
- **Initial-state default.** A state marked `*` becomes the value of `Default*State()`. If no state is marked, the first state in declaration order wins.
- **Internal targets.** `Moving + Tick = _` keeps `s == Moving` while returning `true` from `ProcessEvent`, so wrappers can run side effects without a state change.
- **State and event patterns.** `A | B + Go | Stop = X` produces the cartesian product of source states and events, all targeting `X`.
- **`ValidEvents` exhaustiveness.** Each state's `ValidEvents()` lists specific events first (in source order), then wildcard events that apply because the state has no specific entry for that event.
- **DOT rendering rules.** Wildcards are expanded into explicit per-state edges; the initial state is a `doublecircle`; specific transitions render before wildcards.
- **All four validation rules.** Empty transitions, multiple `*`, duplicate `(state, event)`, duplicate wildcard event. Same messages, same intent.

## Things that change shape

- **`Option<State>` becomes `(State, bool)`.** Go has no `Option` type; the multi-return idiom is exact and just as cheap.
- **`State::default()` becomes `DefaultState()`.** Rust's `Default` trait doesn't exist in Go. The function is a plain package-level factory.
- **`State::ALL` becomes `AllStates`.** Go has no associated consts; the equivalent is a package-scope `var` holding a fixed-size array.
- **`ValidEvents` returns a slice over a package-scope array.** Rust returns `&'static [Event]`. Go has no equivalent of `'static` slices in generic code, so the generator emits one `var` per state holding the backing slice. The runtime cost is the same: a slice header copy on return, no allocation.
- **Errors are `file:line:column: msg` strings.** Rust uses `compile_error!`, which integrates with `cargo`'s diagnostic stream. Go has no equivalent, so the CLI emits the same format that `go build` uses, and editors / CI pick it up the same way.

## Intentionally dropped

- **`derive_states:` / `derive_events:`.** The Rust macro lets you customize the derives on the generated enums (e.g. `[Debug, Serialize, Deserialize]`). Go has no `derive` system. The Go generator unconditionally emits `String()` on both enums; anything else (serde-like JSON, gob, etc.) the consumer adds via separate code or struct tags on a wrapper.

- **`no_std` flag.** The Rust crate notes it works without an allocator. The Go generator's output is allocation-free (the `ValidEvents` indirection exists specifically to keep it that way), but there is no Go equivalent of `no_std` to flag explicitly.

## Things that are new in the Go version

- **`package:` header.** Required, because the generator writes to a file rather than expanding inline.
- **External `.sm` file.** A single source-of-truth file, separate from any `.go` file. This makes it natural to keep the state machine under version control as a first-class artifact rather than an implementation detail of one Go file.
- **CLI parseable error format.** `file:line:column: msg` works with every Go editor's quickfix pane.
