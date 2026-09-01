# Rules engine

The production implementation lives in `internal/rules` and contains
deterministic, host-free D&D computation. `math.go` owns universal arithmetic;
`ruleset.go` validates the complete provider profile and interprets its edition
policy. The worker boundary in `internal/engine` adapts these functions to the
lifecycle-scoped v3 service contract.

The engine contains formulas and generic mechanics, not sourcebook records or
sheet presentation. It never reads CodexHost storage, writes character data,
or branches on provider, book, or product IDs.

## Profile authority

The selected rules-data provider is the sole authority for edition policy. It
must publish a complete profile; the engine has no native profile and rejects
inheritance. Printed class progression remains more specific than a profile
table, while structured class fields add class-specific Builder opportunities.

Hydration returns a computed sheet plus warnings. Ordinary incomplete character
choices degrade to warnings rather than exceptions. The public API clones
inputs and outputs at its boundary so callers cannot mutate another consumer's
state.

The JavaScript modules in this directory remain only as a temporary regression
oracle while hydration and Builder behavior move to Go; they are not included
in the v3 install package.

Run `go test ./...` and the focused race tests documented in the root README
after any production rules change. Until the migration oracle is removed, also
run `node --test tests/*.mjs`.
