# D&D service contracts

The engine provides `dnd5e.rules-engine` 4.0.0 and consumes nonexclusive
`dnd5e.rules-data` 3.0.0. Provider selection, compatible books and generation
lifetime belong to the host. Runtime policy addresses contracts and record IDs.

## Character evaluation

`evaluate-character` accepts `{contractVersion:"rules-character.v1", inputs}`.
`character-play` adds a closed operation-specific `change`. The response is
`{contractVersion:"rules-character-response.v1", identity, evaluation, policy}`.
Requests are bounded to 180,000 bytes. The public Go types in
[character/model.go](../character/model.go) generate the JSON Schemas through
`go run ./cmd/character-contract`; the sheet uses these same types.

Inputs contain ordered levels with stable IDs and recorded HP rolls, foundation
and advancement decisions, spell selections/acquisitions, inventory instances,
spent resource counters, current/temporary HP, notes and typed DM grants.
The caller supplies the evaluation time in `play.asOf`. Repeated evaluation
of the same inputs and sources is deterministic and leaves those inputs intact.

The result includes inputs, projection, decision plan, labeled guidance, spell
options, issues, source evidence and explanations. `ready` means the declared
choices and supported mechanical constraints pass. Incomplete foundations
produce an explicit unknown projection rather than plausible zero statistics.
Unsupported prerequisites require an exact issue-ID waiver from an authorized
DM. Narrative content and encounter effects remain visible source facts.

## Play commands

Commands include damage, healing, direct HP, temporary HP, short/long rests,
recorded hit-die spending, declared feature activation, spell/grant selections,
casting, rituals, copying and level-up spell replacement. Each command validates
its own fields and uses the same evaluation before and after applying it.
An invalid character must be repaired in Build before dependent play actions.

Damage/healing clamp to known bounds; direct invalid HP is rejected. Hit dice
require the actual recorded result and a unique roll ID. Resources store spent
uses; the projection exposes capacity, spent and remaining. Ending an activation
replaces its state map, so removed keys cannot survive as merged old values.
Long rests clear active features through the common rest interpreter.

Copying requires a unique acquisition ID and the profile's GP cost. An optional
owned scroll must explicitly name the spell, and one instance is consumed in
the same detached change. Acquisitions retain cost, character level, time and
scroll identity. Re-evaluation never recreates consumed inventory. A source's
`spellcasting.levelReplacements` grants a bounded number of known-spell
replacements at each class level after first level. Recorded replacements
consume that level's allowance and remain in the ledger across rests.

## Effects, sources and authority

DM and versioned item effects share the typed target/mode vocabulary. Supported
targets are ability score/cap, AC, saving throw, initiative, movement, maximum
HP, attunement capacity, sense range, resource capacity and proficiency.
Conditions are always/equipped/attuned with optional effective level and expiry.
Conflicting absolute replacements, impossible bounds and unknown resources
block activation. Inventory IDs identify individual items. Duplicate magic-item
properties are suppressed deterministically and explained; the profile can
also prohibit attuning multiple copies of the same item definition.

An equipped/attuned magic item requires `characterMechanics: "complete"`
with supported typed effects, an explicitly narrative declaration, or linked DM
adjudication. Reusable homebrew belongs in a compatible versioned source package.

The engine evaluates grant facts but cannot authenticate their origin. The
coordinator must verify editor/DM roles, stamp grant authority, review imports,
preserve history, and commit the exact accepted result atomically. Engine output
never grants persistence authority to a browser.

Evidence identifies each contributing record, book, content hash, bounded
summary, mechanical facts, package ID, archive generation hash and content
revision. The sheet additionally retains the engine's package version/hash.
Missing sources cannot retroactively alter this saved evidence. Current rule
links remain subject to current source and viewer access.

## Provider contract

The worker reaches `catalog`, `get` and `query` through host-issued
all-compatible handles. Exactly one complete rules profile is required across
the enabled providers. Duplicate (kind, ID) records are rejected. Names are
display values, and the character path resolves source records by ID only.
Provider replacement or source-policy revision invalidates the evaluation
snapshot. Returned values are detached from provider caches.

The manifest and [service schema](../contracts/rules-engine.service.json) also
specify context, record queries and derivation. Public character evaluation has
no legacy envelopes or provider-free manually writable projection.
