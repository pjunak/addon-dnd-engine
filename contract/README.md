# D&D service contracts

This repository is the consumer-side compatibility authority for the engine
side of two host-brokered services:

- `dnd5e.rules-data` `3.0.0`: structured rule records and one explicit,
  validated ruleset from a replaceable provider.
- `dnd5e.rules-engine` `3.0.0`: headless deterministic computation and the
  provider-neutral list/get/reference surface used by sheet builders.

Provider selection is handled by CodexHost. Consumers never probe addon IDs.
The selected handle supplies provider package and content revision metadata;
`getContextIdentity()` combines that metadata with the ruleset identity.

## Rules-data v3

The engine reaches the selected provider only through `host/service.call` and
the package-manager-issued `dnd5e.rules-data` handle. It calls the immutable
content methods `catalog`, `get`, and `query`; it never imports provider code or
probes a known add-on ID. A ruleset has stable
`rulesetId`, positive `rulesetVersion`, and `edition`. It must contain every
required computation constant, capability flag, and Builder policy field.
Inheritance is rejected: missing fields never imply an engine edition.
The boundary validates every engine-consumed constant and capability, rejects
non-finite or non-plain data, and retains a detached recursively frozen
snapshot. Later provider mutation therefore cannot change an active engine
context.

`resolveReference(kind, id, mode)` is optional. When present it returns a
provider-owned navigation descriptor; consumers otherwise show an unlinked
label. List projections should be fresh. Full-record identity behavior remains
provider-documented. The engine catches provider access failures, rejects the
wrong list/item shape, and returns detached data from its public surface.

The Go provider tests and [`../testdata/synthetic-ruleset.json`](../testdata/synthetic-ruleset.json)
are the redistributable conformance fixtures. Provider repositories should run
equivalent validation against their real content without copying production
records here.

Record identity is `(kind, id)`. Names are display labels and may repeat within
a kind (for example, features acquired at different levels). Snapshot loading
keeps all such records available by ID. Name-only lookup succeeds only for a
unique trimmed, case-insensitive name; an ambiguous name stays unresolved.
A changed provider generation or content revision rebuilds the entire snapshot
and name index. Cached content cannot conceal a missing provider.

The normalized Builder policy covers point buy, origin ability grants, class
advancement choices, feat categories by level, and category-specific ability
caps. Class-specific extra advancement levels use the structured
`abilityScoreImprovementLevels` record field; feature-name matching is not part
of the contract.

## Engine v3

The package-owned schemas under `contracts/` define all requests and responses.
The surface exposes `context`, record queries, derivation, hydration, and the
Builder plan/apply/reconcile lifecycle. Universal arithmetic does not require a provider. Ruleset-backed
derivations obtain a validated complete profile on the same request and return
the exact provider identity used for the result. Hydration returns a detached
computed sheet plus bounded warnings; when the optional provider is missing it
returns the universal subset rather than inventing edition policy.

Builder calls report `available` and `status` explicitly. Planning requires a
provider; apply and reconcile return the unchanged detached decisions when the
optional provider is unavailable. Hydration normalizes Builder choices before
computing the sheet, so saved decisions and derived state follow one path.

Successful `builder-plan` responses additionally expose optional `guidance`
from the same provider evaluation. It contains labeled `choices` with option
IDs/descriptions, valid picked/required counts, completion and advancement feat
options; `classes` with named level/feature rows and subclass options; and
foundation/progression/spell `sections` with actionable issue targets. The
top-level total/complete/ready fields summarize these declared requirements;
derived stats and warnings are preview values. This is advisory completion of
the provider's declared choices, not a complete character-legality validator.
Neither planning nor guidance changes authored decisions. The existing pure
plan shape is unchanged, and consumers without guidance retain their flat form.

The engine owns no Store, character namespace, UI, routes, or persistence.

`apply-play-change` uses `rules-engine-play-change.v1` requests and
`rules-engine-play-result.v1` responses. Each operation accepts only its named
fields: rest (`rest`), spend-hit-die (`key`), toggle-feature (`key`, `enabled`),
select-spell (`classId`, `ref`, `selection`, `selected`), cast-spell
(`classId`, `ref`, `slot`), select-grant-spell (`key`, `ref`, `selected`),
select-casting-ability (`key`, `ability`), cast-granted-spell (`key`, `slot`),
cast-ritual (`classId`, `ref`), copy-spell (`classId`, `ref`, `scrollId`) and
swap-spell (`classId`, `out`, `ref`). Selection is `cantrips`, `spellbook` or
`preparedSpells`; a cantrip cast uses an empty slot. The service returns both
decisions and the resulting sheet from one evaluation, with its exact identity.
Unavailable rules return unchanged decisions and no computed sheet; invalid
changes fail without a draft. Consumers own confirmation and revisioned writes.
Resolved origin/advancement/reward feats are not promoted into the returned
manual `feats` array. Authored manual entries remain intact, so removing a feat's
original source removes its effect after a play action as well.

Class spells must belong to the class/expanded list and unlocked level; prepared
spellbook spells must be learned first. Removing obsolete references remains
possible. Books can record already learned or copied spells beyond class-level
additions; select-spell records these without charging. Copy-spell instead
deducts the profile's GP cost, rejects insufficient GP or duplicate learning,
and consumes one matching scroll when `scrollId` is nonempty. Empty means a
copy from another book. Scroll identity is an authored `spellRef` or a legacy
name containing both "scroll" and the spell name. The full detached result
must be saved atomically. Swap-spell replaces a non-spellbook class selection
and appends `{level, classLevel, classId, out, in}` to `spellSwaps`; the caller
decides when level-up changes are appropriate.

Standard/pact and restricted feat slots check level, permitted spell list and
remaining uses. Grant keys and casting-ability choice keys come from options;
granted free casts use the existing `charge-<spell>` resource keys. Rituals
require the class's ritual ability and a ritual in its prepared list (or book
for a spellbook class); they consume no slot. Explicitly clearing a default
grant choice stores an empty list so hydration does not select it again.

`spell-options` accepts `rules-engine-spell-options.v1` with `decisions` and
returns `rules-engine-play-result.v1` without applying a play change. The
optional `options` object is also returned after successful play changes:
`classes` contains `classId`, eligible `spellIds`, `ritualIds`, per-spell
`copyCosts` and `castSlots`, plus `canSwap`; `pendingChoices` enriches hydration
choices with `eligibleSpellIds`; `castingAbilityChoices` retains hydration's
choice descriptors; `granted` adds stable `key` and eligible `slots` to each
grant; `slots` contains `key`, `name`, `max` and `current`. Consumers render
these choices rather than duplicating edition policy. Planning and mutation
use one evaluation and return the same exact provider identity.

Rest recharge follows each computed resource declaration. Long rests include
short-rest recovery, restore HP, clear temporary HP and end active features.
Half-level hit-die recovery is shared across pools in stored class order; manual
resources and unrelated authored fields remain unchanged. Hit-die spending uses
the engine's average plus Constitution modifier with at least one HP recovered.
Builder plans also expose the selected feat's ability budget and eligible scores
in the existing nested ability descriptor so consumers need not infer them.

Without rules data it exposes provider-neutral arithmetic and reports
rules-data-dependent work as unavailable instead of selecting a hidden addon or
applying a bundled edition profile.
