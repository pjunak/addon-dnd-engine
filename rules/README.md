# Rules engine

The modules in this directory are deterministic, host-free D&D computation.
`engine.js` interprets generic structured records; `grants.js` collects
source-neutral grants; `ruleset.js` resolves an explicit ruleset; and `api.js`
adapts those pure functions to the lifecycle-scoped service contract.

The engine contains formulas and generic mechanics, not sourcebook records or
sheet presentation. It never reads CodexHost storage, writes character data,
or branches on provider, book, or product IDs.

## Ruleset authority

The native `dnd-2024` object is the explicit standalone context. A provider
must publish a complete ruleset unless it declares compatible inheritance with
`extends: "dnd-2024"`. Another edition does not inherit 2024 fields when they
are absent. Printed class progression remains more specific than a ruleset
table.

Hydration returns a computed sheet plus warnings. Ordinary incomplete character
choices degrade to warnings rather than exceptions. The public API clones
inputs and outputs at its boundary so callers cannot mutate another consumer's
state.

Run `node --test tests/*.mjs` from the repository root after any rules change.
