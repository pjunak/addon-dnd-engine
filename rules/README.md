# Character calculation

The production calculation is pure Go under `internal/rules`. The public
character path starts at `EvaluateCharacter` and `ApplyCharacterPlay`.
It converts typed authored decisions into the shared arithmetic/choice helpers,
then validates progression, items, effects, spells and play bounds. Presentation
and persistence consume its output without implementing edition formulas.

The provider's complete rules profile owns point buy, arrays/roll methods,
level bounds, HP minimum, ability caps, attunement policy, movement units,
spell slots and recovery. Class/source facts supply specific progression,
prerequisites, choices, activations and effects. No book/add-on ID selects math.

Every evaluation builds fresh output. Ordinary source bonuses precede typed
minimum/set effects, and explicitly raised ability caps affect normal increases.
Movement that depends on walking speed is recomputed after item/DM bonuses.
Sense ranges use their greatest applicable contribution, with explicit typed
adjustments afterwards. Armor candidates obey worn-armor eligibility.

Progression checks each acquired class/feat against its earlier state. A feat
cannot qualify itself using its own ability increase. Structured all/any,
ability, level and feature predicates are evaluated; unsupported prose requires
a specific recorded DM waiver. Invalid later choices remain available for repair.

Source facts and explanations are retained separately from compact calculated
class/species/background identities. Numeric rows include their calculation
context; primary statistics include explicit terms, limits and source references.
Applied and suppressed DM/item contributions share one explanation format.

## Verification

Character regressions cover deterministic recalculation, DM revocation/expiry,
ability caps, multiclass acquisition order, HP bounds, recorded dice, rest and
spent counters, attunement, item effects, conditional flight/senses, copying costs
and level replacement budgets. Worker tests exercise the closed v4 boundary.

The 144 pinned v1 arithmetic/Builder vectors remain an independent oracle for
shared helpers. They are synthetic expected-result data. Adapters needed only
by these fixtures live in `*_test.go` files, outside the production package. Explicit tested corrections include class proficiency summaries and
excluding unarmored AC candidates while armor is worn. The installed suites
exercise current v4 characters and package lifecycle separately.

Content coverage is indexed in the provider's [coverage document](../../addon-dnd-2024-compendium/data/COVERAGE.md).
Record prose, tactical adjudication and undeclared effects must not be presented
as fully automated mechanics. New mechanics require source fields, interpretation
and regression evidence together.
