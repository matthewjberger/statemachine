# Releases

`statemachine` is published via Go modules. There is no separate registry step (no equivalent of `cargo publish` or `npm publish`); a tagged commit on the default branch is the release.

## Semver policy

The Go module is at `github.com/matthewjberger/statemachine`. Versioning follows Go's semver rules: `vMAJOR.MINOR.PATCH`. Major version `v2` and above requires a `/v2` module-path suffix; we will reach that bridge if and when we cross it.

A change is **breaking** (requires a major version bump) if it changes the shape of any generated identifier in a way that breaks consumers' code:

- Renaming or removing a public type, function, method, var, or const.
- Changing the signature of a public function or method.
- Changing the DSL syntax in a way that makes existing `.sm` files fail to parse.
- Changing the semantics of an existing DSL construct (e.g. flipping wildcard priority).

A change is **non-breaking** (minor or patch) if it:

- Adds a new DSL construct that doesn't change existing parsing.
- Adds new generated identifiers (e.g. a new helper method on `State`) without removing existing ones.
- Adds a new CLI flag with a default that preserves current behavior.
- Improves error messages, formatting, or comment placement in generated code.
- Fixes a bug where the generator emitted incorrect code for valid input.

The library API (`Parse`, `Validate`, `Generate`, AST types) follows the same rules as a normal Go package.

## Tagging a release

```bash
# Make sure main is clean and CI is green.
git checkout main
git pull
just ci

# Tag and push.
git tag v0.2.0
git push origin v0.2.0
```

That's the entire release. `pkg.go.dev` and the Go module proxy pick up the new tag automatically (usually within minutes). No manual upload is required.

To verify the proxy has cached the new version:

```bash
go list -m -versions github.com/matthewjberger/statemachine
```

If the new tag isn't listed, force a proxy fetch:

```bash
GOPROXY=https://proxy.golang.org go list -m github.com/matthewjberger/statemachine@v0.2.0
```

## Pre-release versions

For testing, tag a pre-release:

```bash
git tag v0.2.0-rc1
git push origin v0.2.0-rc1
```

Consumers opt in explicitly:

```bash
go get github.com/matthewjberger/statemachine@v0.2.0-rc1
```

Standard Go semver pre-release rules apply: `v0.2.0-rc1` sorts before `v0.2.0`.

## Communicating breaking changes

When a release does change generated identifiers or DSL syntax:

1. The release tag bumps the major version (or, for `v0.x` zero-ver releases, the minor version, since `v0.x` has no stability guarantee).
2. The release notes call out the breaking changes explicitly with before/after snippets.
3. The README mentions the latest version compatible with the previous behavior, so consumers who can't migrate immediately know where to pin.

There is currently no CHANGELOG file; release notes live in the GitHub release for each tag.

## Generated-code stability

A subtle case: the **generated `_gen.go` files are committed to consumer repositories.** A change in the generator that produces different (but semantically equivalent) output — a different formatting, a renamed receiver, a reordered switch — is a non-breaking change for the *library*, but it forces consumers to regenerate to get a clean diff on their next `go generate`. We treat such changes as patch-level and call them out in release notes so consumers know to expect a no-op-looking diff after running `go generate`.

If the generated output's externally-observable behavior changes (a method returns a different value, an enum gets reordered), that's a major-version change regardless of how small the source diff looks.
