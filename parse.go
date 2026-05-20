// Package statemachine parses the stateless .sm DSL and generates Go source
// implementing the transition table.
//
// The DSL is intentionally a near-literal port of the Rust `stateless` crate:
//
//	package: traffic
//	name: Light
//
//	transitions: {
//	    *Red + Tick = Green,
//	    Green + Tick = Yellow,
//	    Yellow + Tick = Red,
//	    _ + Reset = Red,
//	}
//
// The package can be used directly as a library (Parse + Generate), or through
// the cmd/statemachine CLI driven by //go:generate.
package statemachine

import (
	"fmt"
	"unicode"
)

// AST is the parsed representation of a .sm file.
type AST struct {
	Package     string
	Name        string
	Transitions []Transition
}

// Transition is one line of the transitions block.
type Transition struct {
	Sources Sources
	Events  []string
	Target  Target
	Pos     Pos
}

// Sources is either a list of named source states or a wildcard ("_").
type Sources struct {
	Wildcard bool
	States   []SourceState
}

// SourceState is one source state, optionally marked as the initial state ("*Name").
type SourceState struct {
	Name    string
	Initial bool
	Pos     Pos
}

// Target is either a named destination state or "_" (internal, stay in current state).
type Target struct {
	Internal bool
	State    string
	Pos      Pos
}

// Pos is a 1-based file position used for error reporting.
type Pos struct {
	Line   int
	Column int
}

// ParseError carries the input filename and a position so it formats as `file:line:col: message`.
type ParseError struct {
	File    string
	Pos     Pos
	Message string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Pos.Line, e.Pos.Column, e.Message)
}

// Parse reads a .sm source and returns the AST. The filename is only used in error messages.
func Parse(filename string, source []byte) (*AST, error) {
	parser := newParser(filename, source)
	return parser.parseFile()
}

type tokenKind int

const (
	tokIdent tokenKind = iota
	tokStar
	tokPlus
	tokEq
	tokPipe
	tokComma
	tokColon
	tokLBrace
	tokRBrace
	tokEOF
)

func (k tokenKind) String() string {
	switch k {
	case tokIdent:
		return "identifier"
	case tokStar:
		return "'*'"
	case tokPlus:
		return "'+'"
	case tokEq:
		return "'='"
	case tokPipe:
		return "'|'"
	case tokComma:
		return "','"
	case tokColon:
		return "':'"
	case tokLBrace:
		return "'{'"
	case tokRBrace:
		return "'}'"
	case tokEOF:
		return "end of file"
	}
	return "unknown"
}

type token struct {
	kind tokenKind
	val  string
	pos  Pos
}

type parser struct {
	file   string
	src    []byte
	offset int
	line   int
	col    int

	peeked    *token
	peekValid bool
}

func newParser(filename string, source []byte) *parser {
	return &parser{
		file: filename,
		src:  source,
		line: 1,
		col:  1,
	}
}

func (p *parser) errorAt(pos Pos, format string, args ...any) error {
	return &ParseError{File: p.file, Pos: pos, Message: fmt.Sprintf(format, args...)}
}

func (p *parser) currentPos() Pos {
	return Pos{Line: p.line, Column: p.col}
}

func (p *parser) advance() {
	if p.offset >= len(p.src) {
		return
	}
	if p.src[p.offset] == '\n' {
		p.line++
		p.col = 1
	} else {
		p.col++
	}
	p.offset++
}

func (p *parser) skipWhitespaceAndComments() {
	for p.offset < len(p.src) {
		ch := p.src[p.offset]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n':
			p.advance()
		case ch == '/' && p.offset+1 < len(p.src) && p.src[p.offset+1] == '/':
			for p.offset < len(p.src) && p.src[p.offset] != '\n' {
				p.advance()
			}
		default:
			return
		}
	}
}

func isIdentStart(ch byte) bool {
	return ch == '_' || unicode.IsLetter(rune(ch))
}

func isIdentCont(ch byte) bool {
	return ch == '_' || unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch))
}

func (p *parser) nextToken() (token, error) {
	p.skipWhitespaceAndComments()
	if p.offset >= len(p.src) {
		return token{kind: tokEOF, pos: p.currentPos()}, nil
	}
	pos := p.currentPos()
	ch := p.src[p.offset]
	switch ch {
	case '*':
		p.advance()
		return token{kind: tokStar, val: "*", pos: pos}, nil
	case '+':
		p.advance()
		return token{kind: tokPlus, val: "+", pos: pos}, nil
	case '=':
		p.advance()
		return token{kind: tokEq, val: "=", pos: pos}, nil
	case '|':
		p.advance()
		return token{kind: tokPipe, val: "|", pos: pos}, nil
	case ',':
		p.advance()
		return token{kind: tokComma, val: ",", pos: pos}, nil
	case ':':
		p.advance()
		return token{kind: tokColon, val: ":", pos: pos}, nil
	case '{':
		p.advance()
		return token{kind: tokLBrace, val: "{", pos: pos}, nil
	case '}':
		p.advance()
		return token{kind: tokRBrace, val: "}", pos: pos}, nil
	}
	if isIdentStart(ch) {
		start := p.offset
		for p.offset < len(p.src) && isIdentCont(p.src[p.offset]) {
			p.advance()
		}
		return token{kind: tokIdent, val: string(p.src[start:p.offset]), pos: pos}, nil
	}
	return token{}, p.errorAt(pos, "unexpected character %q", ch)
}

func (p *parser) peek() (token, error) {
	if p.peekValid {
		return *p.peeked, nil
	}
	tok, err := p.nextToken()
	if err != nil {
		return token{}, err
	}
	p.peeked = &tok
	p.peekValid = true
	return tok, nil
}

func (p *parser) consume() (token, error) {
	if p.peekValid {
		tok := *p.peeked
		p.peekValid = false
		p.peeked = nil
		return tok, nil
	}
	return p.nextToken()
}

func (p *parser) expect(kind tokenKind) (token, error) {
	tok, err := p.consume()
	if err != nil {
		return token{}, err
	}
	if tok.kind != kind {
		return token{}, p.errorAt(tok.pos, "expected %s, got %s", kind, tok.kind)
	}
	return tok, nil
}

func (p *parser) parseFile() (*AST, error) {
	ast := &AST{}

	for {
		tok, err := p.peek()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokEOF {
			break
		}
		if tok.kind != tokIdent {
			return nil, p.errorAt(tok.pos, "expected header key or 'transitions', got %s", tok.kind)
		}

		switch tok.val {
		case "package":
			if _, err := p.consume(); err != nil {
				return nil, err
			}
			if _, err := p.expect(tokColon); err != nil {
				return nil, err
			}
			valueTok, err := p.expect(tokIdent)
			if err != nil {
				return nil, err
			}
			ast.Package = valueTok.val
		case "name":
			if _, err := p.consume(); err != nil {
				return nil, err
			}
			if _, err := p.expect(tokColon); err != nil {
				return nil, err
			}
			valueTok, err := p.expect(tokIdent)
			if err != nil {
				return nil, err
			}
			ast.Name = valueTok.val
		case "transitions":
			transitions, err := p.parseTransitionsBlock()
			if err != nil {
				return nil, err
			}
			ast.Transitions = transitions
		default:
			return nil, p.errorAt(tok.pos, "unknown header key %q (expected 'package', 'name', or 'transitions')", tok.val)
		}
	}

	if ast.Package == "" {
		return nil, &ParseError{File: p.file, Pos: Pos{Line: 1, Column: 1}, Message: "missing required header: package"}
	}
	if ast.Transitions == nil {
		return nil, &ParseError{File: p.file, Pos: Pos{Line: 1, Column: 1}, Message: "missing required block: transitions"}
	}
	return ast, nil
}

func (p *parser) parseTransitionsBlock() ([]Transition, error) {
	if _, err := p.expect(tokIdent); err != nil {
		return nil, err
	}
	if _, err := p.expect(tokColon); err != nil {
		return nil, err
	}
	if _, err := p.expect(tokLBrace); err != nil {
		return nil, err
	}

	var transitions []Transition
	for {
		tok, err := p.peek()
		if err != nil {
			return nil, err
		}
		if tok.kind == tokRBrace {
			if _, err := p.consume(); err != nil {
				return nil, err
			}
			return transitions, nil
		}
		transition, err := p.parseTransition()
		if err != nil {
			return nil, err
		}
		transitions = append(transitions, transition)

		next, err := p.peek()
		if err != nil {
			return nil, err
		}
		if next.kind == tokComma {
			if _, err := p.consume(); err != nil {
				return nil, err
			}
			continue
		}
		if next.kind == tokRBrace {
			if _, err := p.consume(); err != nil {
				return nil, err
			}
			return transitions, nil
		}
		return nil, p.errorAt(next.pos, "expected ',' or '}' after transition, got %s", next.kind)
	}
}

func (p *parser) parseTransition() (Transition, error) {
	startTok, err := p.peek()
	if err != nil {
		return Transition{}, err
	}

	sources, err := p.parseSources()
	if err != nil {
		return Transition{}, err
	}

	if _, err := p.expect(tokPlus); err != nil {
		return Transition{}, err
	}

	events, err := p.parseEventList()
	if err != nil {
		return Transition{}, err
	}

	target, err := p.parseTarget()
	if err != nil {
		return Transition{}, err
	}

	return Transition{
		Sources: sources,
		Events:  events,
		Target:  target,
		Pos:     startTok.pos,
	}, nil
}

func (p *parser) parseSources() (Sources, error) {
	first, err := p.peek()
	if err != nil {
		return Sources{}, err
	}
	if first.kind == tokIdent && first.val == "_" {
		if _, err := p.consume(); err != nil {
			return Sources{}, err
		}
		return Sources{Wildcard: true}, nil
	}

	var states []SourceState
	for {
		initial := false
		tok, err := p.peek()
		if err != nil {
			return Sources{}, err
		}
		if tok.kind == tokStar {
			if _, err := p.consume(); err != nil {
				return Sources{}, err
			}
			initial = true
		}
		identTok, err := p.expect(tokIdent)
		if err != nil {
			return Sources{}, err
		}
		if identTok.val == "_" {
			return Sources{}, p.errorAt(identTok.pos, "'_' wildcard cannot be combined with named states")
		}
		states = append(states, SourceState{Name: identTok.val, Initial: initial, Pos: identTok.pos})

		next, err := p.peek()
		if err != nil {
			return Sources{}, err
		}
		if next.kind != tokPipe {
			return Sources{States: states}, nil
		}
		if _, err := p.consume(); err != nil {
			return Sources{}, err
		}
	}
}

func (p *parser) parseEventList() ([]string, error) {
	var events []string
	first, err := p.expect(tokIdent)
	if err != nil {
		return nil, err
	}
	if first.val == "_" {
		return nil, p.errorAt(first.pos, "events cannot be '_'")
	}
	events = append(events, first.val)
	for {
		next, err := p.peek()
		if err != nil {
			return nil, err
		}
		if next.kind != tokPipe {
			return events, nil
		}
		if _, err := p.consume(); err != nil {
			return nil, err
		}
		ident, err := p.expect(tokIdent)
		if err != nil {
			return nil, err
		}
		if ident.val == "_" {
			return nil, p.errorAt(ident.pos, "events cannot be '_'")
		}
		events = append(events, ident.val)
	}
}

func (p *parser) parseTarget() (Target, error) {
	if _, err := p.expect(tokEq); err != nil {
		return Target{}, err
	}
	ident, err := p.expect(tokIdent)
	if err != nil {
		return Target{}, err
	}
	if ident.val == "_" {
		return Target{Internal: true, Pos: ident.pos}, nil
	}
	return Target{State: ident.val, Pos: ident.pos}, nil
}

// stateOrder returns the deterministic state declaration order across all transitions:
// first appearance wins, scanning sources then targets per transition.
func stateOrder(transitions []Transition) []string {
	var order []string
	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		order = append(order, name)
	}
	for _, transition := range transitions {
		for _, source := range transition.Sources.States {
			add(source.Name)
		}
		if !transition.Target.Internal {
			add(transition.Target.State)
		}
	}
	return order
}

// eventOrder returns the deterministic event declaration order across all transitions.
func eventOrder(transitions []Transition) []string {
	var order []string
	seen := map[string]bool{}
	for _, transition := range transitions {
		for _, event := range transition.Events {
			if seen[event] {
				continue
			}
			seen[event] = true
			order = append(order, event)
		}
	}
	return order
}

// initialState picks the *-marked state if present; otherwise the first state in declaration order.
func initialState(ast *AST) string {
	for _, transition := range ast.Transitions {
		for _, source := range transition.Sources.States {
			if source.Initial {
				return source.Name
			}
		}
	}
	order := stateOrder(ast.Transitions)
	if len(order) > 0 {
		return order[0]
	}
	return ""
}
