# Architecture

This document describes how the `statemachine` code generator turns a `.sm` source file into Go code, end to end. It covers the data flow, every layer of the pipeline, and the rationale behind the choices that look surprising in isolation.

All file paths are relative to the repository root. Line numbers are accurate at the time of writing but may drift, so identifiers are quoted alongside them.

## 1. Top-level layout

The project is a single Go module (`go.mod`) split into three packages:

- The root package `statemachine` exposes the codegen as a library. `parse.go`, `validate.go`, and `gen.go` are the only source files; the public surface is `Parse`, `Validate`, `Generate`, and the AST types.
- `cmd/statemachine` is a thin CLI that wires the library to the filesystem. It exists so users can drive generation through `//go:generate` without depending on the library at compile time.
- `examples/traffic` and `examples/robot` are runnable demos with checked-in `_gen.go` files. They double as smoke tests: `go build ./examples/...` proves the generator's output still compiles.

The library has zero non-stdlib dependencies. Generated code has zero dependencies of any kind (not even the `statemachine` package). A consumer who never touches the CLI again can delete the `.sm` and keep the `_gen.go`.

## 2. Pipeline overview

```
.sm bytes
   │
   ▼
┌────────────┐    ┌──────────────┐    ┌───────────────┐    ┌──────────────┐    ┌───────────────┐    ┌──────────────┐
│   lexer    │ -> │   parser     │ -> │   *AST tree   │ -> │  Validate    │ -> │  buildModel   │ -> │  writeFile   │
└────────────┘    └──────────────┘    └───────────────┘    └──────────────┘    └───────────────┘    └──────────────┘
parse.go:195       parse.go:272         parse.go:25–64       validate.go:24       gen.go:70           gen.go:242
                                                                                                              │
                                                                                                              ▼
                                                                                                       go/format.Source
                                                                                                              │
                                                                                                              ▼
                                                                                                       formatted Go bytes
```

The public entry points are exactly the three transitions in the middle row: `Parse` (`parse.go:78`), `Validate` (`validate.go:24`), and `Generate` (`gen.go:12`). The CLI in `cmd/statemachine/main.go` chains them and adds filesystem I/O around the edges.

## 3. Lexer

The lexer lives inside the `parser` struct in `parse.go`. It is hand-rolled and single-pass; there is no separate `lexer` type. `nextToken` (`parse.go:195`) is the workhorse, returning one `token` per call and advancing the cursor.

Tokens are an integer enum (`tokenKind` at `parse.go:83`):

| Kind          | Source     |
|---------------|------------|
| `tokIdent`    | letters, digits, underscores (Unicode letters via `unicode.IsLetter`) |
| `tokStar`     | `*` (initial-state marker)         |
| `tokPlus`     | `+` (state-event separator)        |
| `tokEq`       | `=` (target separator)             |
| `tokPipe`     | `\|` (alternation in states or events) |
| `tokComma`    | `,` (transition separator)         |
| `tokColon`    | `:` (header value separator)       |
| `tokLBrace`   | `{`                                |
| `tokRBrace`   | `}`                                |
| `tokEOF`      | end of input                       |

Whitespace and `//` line comments are skipped in `skipWhitespaceAndComments` (`parse.go:171`). The lexer tracks line and column in `p.line` / `p.col`, advanced by `advance` (`parse.go:158`), so every token carries an accurate `Pos` for error reporting.

The bare underscore `_` is **not** a distinct token kind. The lexer returns it as `tokIdent` with value `"_"`. The parser is what assigns it special meaning (wildcard source or internal target), depending on context. This keeps the lexer trivial and pushes the policy decision to where it belongs.

One-token lookahead is implemented by `peek` / `consume` (`parse.go:238`, `parse.go:251`). The parser never needs more than one token of lookahead.

## 4. Parser

`parseFile` (`parse.go:272`) is the top-level grammar rule:

```
File          := Header* TransitionsBlock?
Header        := "package" ":" Ident
               | "name"    ":" Ident
TransitionsBlock := "transitions" ":" "{" (Transition ","?)* "}"
Transition    := Sources "+" EventList "=" Target
Sources       := "_"
               | ("*"? Ident) ("|" "*"? Ident)*
EventList     := Ident ("|" Ident)*
Target        := Ident       // including "_" for internal
```

The grammar is intentionally permissive about whitespace and trailing commas. Both of the following parse identically:

```
*Idle + Start = Running, Running + Stop = Idle,
```

```
*Idle + Start = Running,
Running + Stop = Idle
```

After parsing, two header-level invariants are checked in `parseFile`:

- `package:` is required. Missing it produces `missing required header: package` at `parse.go:323`.
- `transitions:` is required. Missing the block produces `missing required block: transitions` at `parse.go:326`.

Everything else (multiple `*`, duplicate transitions, duplicate wildcard events) is left for the validator.

### 4.1 AST

The AST is defined at the top of `parse.go`:

- `AST` (`parse.go:26`) holds `Package`, `Name`, and a flat `[]Transition`.
- `Transition` (`parse.go:33`) holds `Sources`, `Events []string`, `Target`, and a `Pos`.
- `Sources` (`parse.go:41`) is a tagged union: either `Wildcard: true` or `States []SourceState`. The two cases are disjoint; the parser enforces that mixing `_` with a named state is an error (`parse.go:445`).
- `SourceState` (`parse.go:47`) carries `Name`, `Initial`, and `Pos`. `Initial` is `true` only for the state immediately following a `*`.
- `Target` (`parse.go:54`) is also a tagged union: `Internal: true` for `_`, otherwise `State` is set.

Every node carries a `Pos`. This is what lets validation and codegen errors report a `file:line:column` location even though the AST is decoupled from the source bytes.

### 4.2 Why a hand-rolled parser

The DSL is small enough (one statement form, one block) that a generated parser would be overkill. The grammar fits in twenty lines and never needs more than one token of lookahead. The hand-rolled approach also gives us precise control over error messages — `parser.errorAt` (`parse.go:150`) formats every error with the source filename and the token's exact position.

## 5. Validator

`Validate` (`validate.go:24`) takes a parsed AST and applies the four structural rules that come straight from the Rust `stateless` proc-macro:

1. **At least one transition.** Empty blocks were accepted by the parser; rejected here.
2. **At most one `*`.** Counted by scanning every `SourceState` across every transition. The second `*` reports its own position.
3. **No duplicate `(state, event)` pair.** A `pair` is a 2-tuple of strings, looked up in a `map[pair]Pos`. Wildcards are not expanded for this check — they have their own rule below.
4. **No duplicate wildcard event.** A `_ + Reset = A, _ + Reset = B` definition is rejected because the runtime would have no way to pick one.

Validation runs in O(transitions × max(sources, events) × max source count). For any realistic state machine this is microseconds.

Errors are `*ValidationError` (`validate.go:7`), which formats identically to `*ParseError` (`file:line:col: message`). Both implement the standard `error` interface, so callers do not need to know which layer rejected the input.

The validator is **only** structural. It does not check that the target of a transition matches an actual source state somewhere else (that would forbid a state that only ever appears as a target — sometimes useful, e.g. a terminal `End` state declared only as the right-hand side of a `Go = End`). The state set is derived from union of all sources and all targets in `stateOrder` (`parse.go:509`).

## 6. Codegen

`Generate` (`gen.go:12`) drives the back end. It does two things:

1. Call `buildModel` (`gen.go:70`) to turn the AST into a flat `*model` ready for emission.
2. Walk `writeFile` (`gen.go:242`), then run the buffered output through `go/format.Source`.

The split between the model and the emitter is the most important design choice in the back end. The model is a deliberate flattening of the AST — name resolution, dedup, wildcard expansion, and DOT pre-rendering all happen there. The emitter is dumb: it walks the model in fixed order and writes strings. Everything that requires understanding the DSL semantics lives in `buildModel`; everything that requires understanding Go syntax lives in the `write*` functions. If a generated-code change is purely cosmetic (rename a method, change a receiver letter), only the emitter needs touching.

### 6.1 The model

`model` (`gen.go:26`) is the canonical view of what gets emitted:

- `Package`, `StateType`, `EventType`, `StatePrefix`, `EventPrefix`, `DefaultFunc`, `AllStatesVar`, `AllEventsVar`, `DOTConstName`: all resolved names. If `Name == ""`, these collapse to `State`, `Event`, `StateRed`, etc. If `Name == "Light"`, they become `LightState`, `LightEvent`, `LightStateRed`, etc. Name resolution lives in lines `75–92` of `gen.go`.
- `States`, `Events`: declaration order, deduplicated. Sourced from `stateOrder` and `eventOrder` (`parse.go:509`, `parse.go:531`).
- `Initial`: the `*`-marked state, falling back to the first state in declaration order via `initialState` (`parse.go:547`).
- `Specific`, `Wildcard`: transitions flattened into one row per `(source, event)`. Wildcards are kept separate so the emitter can render them in a second switch *after* the specific switches.
- `ValidEvents`: one entry per state, with the events that produce a valid transition from that state. Wildcard events are merged in only for states that don't have a specific transition for that event. This matches the Rust crate's behavior exactly.
- `DOT`: the Graphviz output as a single string. Pre-rendered here (not in the emitter) so that the emitter doesn't need to know which states are reachable via wildcards.

### 6.2 The emitter

`writeFile` (`gen.go:242`) is fifteen lines and lists every output construct in order:

1. `// Code generated` header (recognised by `go vet` and most CI gates).
2. `package` declaration.
3. State enum (`writeEnum` → `type ... uint8`, then `const (...)` with `iota`).
4. State `String()` (`writeStringer`).
5. State `All*` array (`writeAllVar`).
6. Event enum / String / All*, same three helpers.
7. `Default*State()` (`writeDefault`).
8. `(StateType).ProcessEvent` (`writeProcessEvent`).
9. Package-scope `valid_events` slices (`writeValidEventsTables`).
10. `(StateType).ValidEvents` (`writeValidEvents`).
11. `DOT` const (`writeDOT`).

The order is significant for readability of the generated file but not for correctness — Go's package-scope identifiers are visible regardless of declaration order.

### 6.3 Specific-over-wildcard ordering

`writeProcessEvent` (`gen.go:300`) emits two top-level switches in sequence:

```go
switch s {
case StateA:
    switch event { ... }
case StateB:
    switch event { ... }
}
switch event {        // wildcards
case EventReset:
    return StateA, true
}
return s, false
```

A specific `(source, event)` pair returns first, so wildcards always lose to a specific match for the same state-event pair. This mirrors the Rust crate's `transition_checks.extend(wildcard_checks)` line, where wildcards are appended after specifics so the `if ... { return ... }` chain naturally prefers earlier specific matches.

### 6.4 ValidEvents tables

`writeValidEventsTables` (`gen.go:342`) emits one `[]EventType` package-scope slice per state. `writeValidEvents` (`gen.go:360`) then switches on the state and returns the pre-built slice. The reason for the indirection is to make `ValidEvents` allocation-free: returning a slice literal `[]Event{...}` on every call would allocate fresh backing storage. Naming follows `lowerFirst(StatePrefix) + StateName + "ValidEvents"` (`gen.go:374`).

### 6.5 DOT generation

`buildDOT` (`gen.go:201`) builds the DOT string in two passes that mirror the generator's wildcard-expansion logic:

1. Specific transitions: one `"Src" -> "Dst" [label="Event"]` edge per `(source, event)` pair from a non-wildcard transition.
2. Wildcard transitions: for every state in declaration order, for every wildcard event, emit an edge — **unless** there is already a specific `(state, event)` pair, which would override the wildcard.

The result is a `digraph` with the initial state rendered as a `doublecircle`. The string is emitted by `writeDOT` (`gen.go:370`) as a raw-string `const`. Because Go's linker dead-codes unused `const` strings, projects that never reference `DOT` pay zero binary cost.

### 6.6 Formatting

`Generate` runs the buffered output through `go/format.Source`. If formatting fails (which it shouldn't, because the emitter only writes well-formed Go), the error wraps the original bytes so the caller can see what was emitted and diagnose the bug.

## 7. The CLI

`cmd/statemachine/main.go` is a thin wrapper:

- `main` (`main.go:23`) parses one positional argument (the `.sm` path) and one optional `-out` flag.
- `run` (`main.go:43`) chains `os.ReadFile` → `Parse` → `Validate` → `Generate` → `os.WriteFile`. Any error short-circuits and is printed to stderr.

The default output path is `<input>_gen.go` in the same directory as the input, computed by trimming the extension. This convention is what lets a typical `//go:generate statemachine traffic.sm` directive work without any flags.

The CLI does not know about Go modules, `go env`, or `GOBIN`. It is invoked once per `.sm` file by the user's `go generate` and exits.

## 8. Testing strategy

Tests live alongside the source: `parse_test.go`, `validate_test.go`, `gen_test.go`. The strategy at each layer:

- **Parser tests** assert AST shape after parsing. Test cases cover every grammar production at least once (state pattern, event pattern, wildcard source, internal target, comments, trailing comma) plus the three error positions (missing package, missing transitions, unexpected character).
- **Validator tests** construct an AST via `Parse`, then assert that `Validate` rejects (or accepts) it with the expected error message substring.
- **Generator tests** are substring-matching against the output of `Generate`. There is no golden-file snapshot — substring matching survives cosmetic format changes (extra blank lines, gofmt rule shifts) without needing manual snapshot updates.

The examples (`examples/traffic`, `examples/robot`) also serve as integration tests: `go build ./examples/...` proves that the checked-in `_gen.go` files compile against the demo `main.go` wrappers, end to end.

## 9. Extending the generator

A typical new feature touches every layer of the pipeline. The pattern, in order:

1. **Lexer**: if the feature needs a new token (rare — the existing punctuation set is rich), add a `tokenKind` and a case in `nextToken`.
2. **Parser**: extend the grammar in `parseFile` or one of the `parseX` helpers. Add a field to the AST if needed.
3. **Validator**: add a new rule in `Validate` if the feature can be misused.
4. **Codegen**: extend `model` and `buildModel`, then add or modify a `write*` function in the emitter.
5. **Tests**: add cases at each layer above where you changed behavior.
6. **Examples**: if the feature is user-visible, demonstrate it in `examples/`.
7. **Docs**: update `docs/DSL.md` and `docs/CODEGEN.md`.

The model-then-emitter split means most features can be added without writing any new emission logic — just enriching the model and tweaking a single `write*` function.
