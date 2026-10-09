# Character calculation

The production calculation is pure Go under `internal/rules`. The public
character path starts at `EvaluateCharacter` and `ApplyCharacterPlay`.
It converts typed authored decisions into the shared arithmetic/choice helpers,
then validates progression, items, effects, spells and play bounds. Presentation
and persistence consume its output without implementing edition formulas.

The provider's complete rules profile (ruleset version 4 or later) owns point
buy, arrays/roll methods, level bounds, HP minimum, ability caps, attunement
policy, movement units, spell slots and recovery, and the core formulas: the
proficiency bonus by level, the spell save DC base, the passive check base,
the unarmored AC base and the fixed hit-point bonus (half the die plus it).
Only the ability modifier, clamping and the first level's maximum hit die are
engine arithmetic. Class/source facts supply specific progression,
prerequisites, choices, activations and effects. No book/add-on ID selects math.

Every evaluation builds fresh output. A request-local decoded record cache reuses
immutable feature catalogs and Builder source lookups for read-only work;
mutable lookups and nested values copied into Builder output remain detached.
The cache is discarded after evaluation, so provider and source-policy changes
cannot reuse an earlier calculation's records. Character lookups use IDs only. Ordinary source bonuses precede typed
minimum/set effects, and explicitly raised ability caps affect normal increases.
Movement that depends on walking speed is recomputed after item/DM bonuses.
Sense ranges use their greatest applicable contribution, with explicit typed
adjustments afterwards. Armor candidates obey worn-armor eligibility. Empty inventory instances may
remain carried or stored, but cannot stay equipped or attuned. Custom equipment
requires matching active DM mechanics at the current level and evaluation time.
Equip/attune guidance evaluates the proposed state, including item-conditioned
grant authority, without changing the caller's inventory. Validation and
projection share source-declared armor/shield slots. Attunement prerequisites
exclude the candidate's own attunement benefits and conditional waivers;
capacity warnings use the final authorized limit. Pure regressions cover
nonstandard record IDs/kinds, duplicate slots/copies, capacity grant withdrawal,
self-qualification, expired/revoked prerequisites and explicit DM adjudication.

Progression checks each acquired class/feat against its earlier state. Normalized
feat references retain their IDs through these checks and option filtering. A feat
cannot qualify itself using its own ability increase. Source-declared conditional
repetition reserves distinct enumerated choices per acquisition, bounds repetition
by the finite pool, and rejects unknown policies. Missing choices can save;
duplicates and later choices remain repairable through ordinary Builder guidance. Structured all/any,
ability, level and feature predicates are evaluated; unsupported prose requires
a specific recorded DM waiver. Invalid later choices remain available for repair.
Editor options reuse the same acquisition-order checks, restricted to the class
or candidate feat issues they consume. Saved characters still receive all
progression checks. Empty prerequisites skip sheet hydration without bypassing
repeatability checks. Progression normalizes earlier decisions only when a new
relevant prerequisite needs them. Applying authored choices reuses the
calculation's private decision map; the public Builder operation still detaches
its input. Calculations never cache eligibility across requests.

Source facts and explanations are retained separately from compact calculated
class/species/background identities. Numeric rows include their calculation
context; primary statistics include explicit terms, limits and source references.
Applied and suppressed DM/item contributions share one explanation format.

Body placement is independent authored organization. Only current source facts
or an active linked custom-item ruling supply placement choices. Multiple items
may share a display group without bypassing armor/shield or attunement limits.
Validation retains invalid assignments for explicit repair; calculation and
rest never move them. Consuming the final unit clears placement atomically.
Pure cases cover Face/Legs facts from arbitrary source IDs, multiple accessories,
malformed/withdrawn declarations, custom authority, incomplete builds, detached
results and unchanged statistics.

## Held equipment

The optional hand model uses source facts: `damage`, `properties`
(`two-handed`, `versatile`), `versatileDamage`, and `armorType: "shield"`.
No record or provider ID supplies a rule. A selected versatile weapon uses its
two-handed die in both the attack row and saved explanation; selected rows
retain the owned `itemId` and `grip`, including when multiple copies share a
source record. Equipped hand items consume at most two hands once this model is
authored; required-two-handed weapons consume both. Characters without hand
state keep their existing projection and inventory behavior.

Suspension excludes attacks, shield AC, item effects and item-conditioned DM
effects, even if another editor moves the suspended instance. It does not
release attunement capacity. Restoration requires the exact post-suspension
item fingerprint and current source eligibility. Changing even its notes
requires an explicit subsequent hand selection instead of automatic restoration.

The [official 2024 equipment rules](https://www.dndbeyond.com/sources/dnd/br-2024/equipment)
inform the supported grip/damage and shield behavior. This is equipment state,
not action or encounter simulation: shield don/doff costs, attack timing and
conditional mounted exceptions are not automated. Undeclared item mechanics
remain outside the supported hand options.

## Condition tracking

Condition IDs, level limits and supported effects come from the profile-selected
rule record. Optional authored selections stay detached and survive ordinary
evaluation/rest. Movement restrictions apply after other bonuses; zero Speed
wins and all results have a floor of zero. The D20 penalty remains an explicit
roll adjustment, separate from base statistics and spell save DCs. Existing
condition immunities suppress effects without discarding tracked selections.
This is bounded sheet guidance, not expiry, targeting or encounter resolution.
See the [condition contract](../contract/README.md#authored-conditions).

## Verification

Character regressions cover deterministic recalculation, DM revocation/expiry,
ability caps, multiclass acquisition order, HP bounds, recorded dice, rest and
spent counters, attunement, item effects, conditional flight/senses, copying costs
and level replacement budgets. Worker tests exercise the closed v4 boundary.

`go test ./internal/rules -run '^$' -bench BenchmarkMulticlassCharacterCatalog -benchmem`
measures a synthetic multiclass calculation with a larger choice catalog.
Ownership regressions mutate Builder results and reuse the same record snapshot
to check that caller decisions and borrowed nested records stay separate.

`testdata/regression-vectors.json` pins complete Hydrate, Builder plan and
choice results for 143 synthetic 2014 and 2024 characters. After a deliberate
behaviour change, regenerate it with
`go test ./internal/rules -run TestRegressionVectors -update` and review the
diff. The installed suites exercise current v4 characters and package lifecycle
separately.

Content coverage is indexed in the provider's [coverage document](../../addon-dnd-2024-compendium/data/COVERAGE.md).
Record prose, tactical adjudication and undeclared effects must not be presented
as fully automated mechanics. New mechanics require source fields, interpretation
and regression evidence together.
