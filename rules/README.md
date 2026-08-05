# Rules engine

The modules in this directory are deterministic, host-free D&D computation.
`engine.js` interprets generic structured records; `grants.js` collects
source-neutral grants; `builder.js` normalizes decisions and choice
descriptors; `ruleset.js` requires an explicit provider profile; and `api.js`
adapts those pure functions to the lifecycle-scoped service contract.

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

Run `node --test tests/*.mjs` from the repository root after any rules change.
