# Parity with the AIgentFlow reference validator and the JavaScript port

This document defines how this Go implementation stays in step with two things:

1. the **AIgentFlow Go engine's static checks** — the ultimate reference, closed-source; and
2. **[`aigentflow-flow-validator-js`](https://github.com/itsatony/aigentflow-flow-validator-js)** —
   the MIT-licensed JavaScript port this library was ported *from*, and whose `PARITY.md` maps every
   rule back to the reference.

**Tracks AIgentFlow flow schema: `v2.485.0`** (`specVersion` in
[`spec/aigentflow-spec.json`](./spec/aigentflow-spec.json)).

## The comparison contract

**Error `code` + the `valid` verdict.** Never message wording. `Message`, `Context`, and
`Suggestion` are for humans and may differ freely between implementations; `Code` is what a consumer
branches on, so it is the thing held stable.

`Valid` is false ⇔ `Errors` is non-empty. Warnings never affect it, in either implementation.

## How the two are kept in step

Three mechanisms, in increasing strength. All three live in `parity_test.go` and skip cleanly when
the JS checkout is absent, so CI on a bare clone passes — but they fail loudly when it is present and
the two disagree, which is the only way a one-sided change gets caught.

1. **`TestParitySpecIsByteIdenticalUpstream`** — `spec/aigentflow-spec.json` must be byte-identical
   to the JS repo's `src/spec/aigentflow-spec.json`. This is why the enum surface is **embedded via
   `go:embed` and never transcribed into Go constants**: an upstream enum bump stays a one-file copy
   that the conformance suite immediately re-verifies, instead of 51 hand-edited string literals
   nobody re-reads.
2. **`TestParityFixtureSetIsIdentical`** — the conformance corpus (`testdata/conformance/`) must
   match the JS repo's `test/conformance/fixtures/` by name *and* by bytes. A fixture edited on one
   side only would let the two suites be self-consistently green about different documents.
3. **`TestParityVerdictsMatchTheJSImplementation`** — runs the JS CLI over every shared fixture and
   diffs the code sets. Error codes must match **exactly**; JS warnings must all be present here
   (not vice versa — see the intended divergences below).

Mechanism 3 depends on a **fresh** JS build. The harness reads the built CLI's own reported schema
version and skips with an explanatory message when it trails this library's, because a stale
`dist/` would otherwise surface as parity drift. `make parity-check` rebuilds first for that reason.

## Intended divergences

These are deliberate and are *not* parity failures. Each states its direction, because the
conformance harness's subset assertions only tolerate one direction.

### 1. Template syntax: this implementation is exact where JS approximates

The JS port hand-rolls a lexer (`src/template/gotmpl-syntax.ts`) and says so plainly: "NOT a renderer
and NOT a full parser… deliberate bias against false positives". JavaScript has no Go template
parser, so it cannot do better.

Go does: this library calls `text/template.Parse` — the *same parser the AIgentFlow engine uses* —
with every one of the allow-listed template functions registered as a no-op stub. Delimiters,
quoting, comments, `if`/`range`/`with`/`block`/`define` nesting, `else` placement, and empty actions
are therefore judged exactly rather than approximately.

**Direction: Go ⊇ JS on `template_syntax_error`.** Go may reject a pipeline the JS lexer waves
through; it will not accept one JS rejects. The verdict harness asserts error-code *equality* on the
shared fixtures (which both implementations agree on today) and warning *containment*; a future
fixture where Go is legitimately stricter must be added to both repos, with the JS side's expectation
recorded as a known gap in *its* `PARITY.md`.

Unknown function names are discovered by registering a stub and re-parsing in a bounded loop, so an
unknown function never masks the rest of a document's syntax errors — a single-pass parse would stop
at the first unrecognised name.

### 2. `pattern` compilation: RE2, not the JS regex engine

`input_schema[].pattern` is compiled with Go's `regexp` (RE2), which is what the engine uses. The JS
port compiles with the JS regex engine. A pattern valid in one and not the other diverges; **this
side is authoritative**, since it matches the runtime.

### 3. Durations: `time.ParseDuration`, not a hand-rolled scanner

The JS port reimplements Go's duration grammar (including the `µs`/`μs` variants). This library calls
`time.ParseDuration` directly, so the check is exact. No known behavioural difference; recorded
because it is a different mechanism reaching the same rule.

### 4. Finding ORDER differs

Go map iteration is randomised, so every validator here walks steps and mappings in **sorted key
order**; the JS port walks YAML insertion order. Order is not part of the comparison contract (code
*sets* are), and determinism is worth more than insertion order to a consumer that diffs or caches
verdicts — `TestDeterministicFindingOrder` locks it.

### 5. Source positions are an addition

The JS port positions parse errors only. This library builds a path→position index from the
`yaml.Node` tree and resolves **every** finding to a line and column, falling back to the nearest
existing ancestor for a path that by definition does not exist (a `missing_required_field` names a
key that is absent — landing an author on the enclosing step is right, landing them on line 1 is
useless). Purely additive: no verdict depends on it.

### 6. Not ported: the JS CLI

`src/cli.ts` has no counterpart. The Go consumer is a library.

## Enum surfaces carried but not yet consumed

The vendored spec includes values no static rule reads yet: `parallelResolutions`,
`inputSchema.datePattern`, `inputSchema.maxInputKeyCount`, `inputSchema.maxStringInputLength`, and
`evalJudgeUrl`. They are unused in the JS implementation too — the reference validates them at
runtime, not statically. They stay in the embedded document so it remains byte-identical upstream;
**do not prune them**, that would break mechanism 1.

## Discipline for a schema bump

When AIgentFlow's flow schema changes:

1. Land it upstream in the JS repo first (its `PARITY.md` §"Discipline" governs that side), including
   the new enum values and any new conformance fixture.
2. `cp <js-repo>/src/spec/aigentflow-spec.json spec/aigentflow-spec.json` — never hand-edit either copy.
3. `cp <js-repo>/test/conformance/fixtures/*.yaml testdata/conformance/` and add the matching case to
   `conformanceCases`. `TestConformanceFixturesAreAllCovered` fails if you forget the case, so a copied
   fixture cannot sit unasserted.
4. Port the new rule, with its `code` constant added to `keys.go`.
5. `make ci && make parity-check`.
6. Note the version and the ported rule at the top of this file's tracked-version line.

## Consumers that BLOCK on a verdict

If you are using this as an admission gate (aigentverse does), two obligations follow from the
pinned-schema model:

- **Surface `Result.SpecVersion`** on every rejection. See README.
- **Keep `StrictRegistries` off.** The registry-lag warnings exist precisely so a real-but-newer
  executor scheme, template function, or orchestrator tool cannot block a publish.
