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

Run `go test ./...` and the focused race tests documented in the root README
after any production rules change.

## Preserved-engine comparisons

`testdata/v1-parity.json` records full JSON results from the v1 engine at
`b2ba2be2940d63fe6c7d769772773540748a6355`, together with hashes of its source
and redistributable synthetic fixtures. `tools/generate-v1-vectors.mjs` reads
only that pinned Git revision into a temporary directory, runs its 13 engine
tests, and produces 144 vectors. Neither a current Go result nor production
Compendium records generate the expected results. Normal Go tests need no
Node runtime or archived checkout.

The vectors compare both editions, class levels and feature unlocks, ordered
multiclasses, pact and ordinary spell slots, species and lineage grants,
armor and weapons, feat spell choices, mastery, ability caps, Builder plans,
ability/feat changes, reconciliation, and the missing-provider fallback.
The Go comparison also checks that evaluation leaves authored input unchanged.
Hit-die resource order and absent optional Builder fields follow v1.

One deliberate difference is recorded per affected vector: the Go sheet keeps
class weapon proficiencies in its proficiency summary. V1 used those same
tokens to compute weapon attacks but omitted them from the displayed/saved
summary. The test checks the original empty v1 field before substituting the
exact reviewed tokens; every other field is compared without filtering.

The host's installed rules test additionally loads reviewed Engine, Sheets and
Compendium ZIPs, hydrates every installed class, applies Builder choices,
replaces a disposable provider with changed content, and verifies missing data
and reactivation. This checks the service and package boundary separately from
pure computation; sheet presentation remains the Sheets product gate.
