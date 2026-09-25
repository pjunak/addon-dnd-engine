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
Per-item `equipment` guidance supplies equip/attune eligibility and exclusive armor slots.
The guidance includes `canSave` for legal incomplete builds and `saveIssues`,
an ordered array of issue objects explaining values that prevent saving. Required
choices that may legally remain unfinished are excluded from `saveIssues`;
`ready` still requires completion for play. `classOptions` supplies eligible
next classes. Choice
options include prerequisites and descriptions; the plan supplies point-buy
costs and bounds.

Expertise options use the proficiencies available at the granting character
level, including ordered multiclass acquisition and active DM proficiency
grants. Fixed Expertise and valid earlier selections exclude duplicate skills;
the current grant does not invalidate itself. Same-level creation choices
precede class choices, with stable ID order within each group. A declared
`changeOn` replacement may use current proficiencies. Combined
`skillExpertise` grants supply proficiency themselves. Guidance and validation
use the same option pool, and invalid slots retain exact
`invalid-option:<choice-id>#<slot>` identities for the coordinator's existing
slot repair behavior.

Repeatable feats supply separate `grants.choices` descriptors per granting
background/species, choice slot, advancement or DM grant. IDs have the shape
`feat:<feat-id>@<query-escaped-acquisition-id>:<local-choice-id>`; deleting one
source never renumbers another. Nonrepeatable feat IDs remain unchanged.
Acquisitions are discovered from declared choices, never arbitrary input-key
suffixes. Parents resolve before dependent selections. Guidance labels retain
the granting source, and each acquisition is checked for prerequisites;
nonrepeatable duplicates are rejected.

Typed skill/tool and mixed proficiency options share canonical eligibility.
Fixed proficiencies and earlier valid selections are excluded at acquisition
time. Within a level, class choices and direct origin choices precede granted
feat choices. The current acquisition keeps its own selections; duplicate slots
and later repeated selections block saving. Combined skill/Expertise training
can improve an existing proficiency. Source-declared replaceable training uses
current proficiencies after permanent choices. Individual tools and
instrument/game variants remain provider-owned IDs.

A single historical unscoped repeatable choice normalizes to its sole owner in
the detached `evaluation.inputs`. Ambiguous ownership produces
`ambiguous-feat-choice:<id>#<slot>` and retains the original input for explicit
assignment. Consumers must not treat this issue as automatic withdrawal.
Conditional repetition supports the provider's `repeatable: { by: "choice",
choice: "<local-choice-id>" }` shape. It refers to one finite, single-slot
`enumerated` grant choice. Acquisition order follows character level prefixes;
same-level owners use stable acquisition IDs. Earlier valid selections reserve
their value, while later duplicates receive the existing slot-specific
`invalid-option` issue and repair guidance. Removing an owner releases its value
without renaming other choices. Missing picks remain legal incomplete builds;
more acquisitions than the declared pool permits block saving and are not offered.
Unknown conditional policies cannot authorize repetition. The plan marks the
owning descriptor `distinctAcrossAcquisitions: true`; consumers use its ordinary
options and completion guidance. This does not expand repeated spell/resource
mechanics or combat automation into new contract promises.

Class, subclass and feature choices can be supplied by a selected
`grants.choicePackages` branch. Builder discovery, acquisition validation and
hydration share that resolution. Enumerated parents resolve before dependent
feat choices even when their IDs sort later. Inactive branches contribute no
feats or spells; their authored values remain intact in the Engine result with
exact withdrawal issues for the coordinator. Class/subclass level requirements,
feat categories and nonrepeatable acquisition checks still apply.

A granted spell's optional source-declared class membership is retained as
`source.classId` without replacing the granting record identity. Its selected
list and casting ability remain independent facts. This is current-state build
editing; dedicated level-up replacement actions for class-granted choices are
not supplied by this contract.

An empty advancement `featCategories` list with no level-specific categories
means no category restriction; each option still needs its own prerequisites.
Level-specific categories are additive to the base list. Eligibility uses the
character level at that exact ordered acquisition, including multiclass levels,
not today's final level or the class level alone. Class, feature, ability and
spellcaster predicates use the calculated acquired state in both option filtering
and save validation. Narrative predicates still require an exact DM waiver.
Explicit feat ability caps remain owned by the source record/profile. When a
replacement feat supplies no ability increase, the old assignment is reported as
an `unavailable-choice`; the coordinator can withdraw only its previously saved
value through ordinary dependent-choice repair. New illegal assignments remain
blocked.

Subclass feature rows resolve inline local IDs to canonical records within the
owning class/subclass. Record levels govern acquisition; unrelated or future
features never satisfy a prerequisite. Inline-only features remain readable.
The additive `sheet.feats` array retains each acquired feat's `id`, `name` and
`count`, with prose in the existing bounded evidence. It contains acquired feats,
not the option catalog. Consumers can render saved feat details without live
rules; older projections may omit this array.

Builder sections count required decisions, including the first class and class
cantrips/spellbook selections. Their issues carry a stable `id` target and `tab`.
Invalid existing foundation/advancement choices have `repair: true` and do not
count as complete. This does not remove the input or make a rejected choice
saveable. Labels retain readable `label` text; additive `labelKey` templates
and ordered `labelArgs` let clients translate Engine UI wording while preserving
authored record names. Clients may fall back to `label`.

Choice validation issues use `<kind>:<choice ID>#<slot>` identities. In particular,
`unavailable-choice`, `invalid-option` and `choice-count` identify the exact
withdrawn selection. The `target` is the choice group, not permission to remove
every slot in that group.

Unsupported prerequisites require an exact issue-ID waiver from an authorized
DM. Narrative content and encounter effects remain visible source facts.

## Authored Inspiration

The optional boolean `inputs.play.inspiration` records the current allocation.
Omission remains valid for earlier characters and displays as unavailable;
explicit `false` records spending it. Evaluation does not populate an omitted
input, and calculation, rests and other play commands preserve its value.
This is authored state, with no automatic award, reroll or encounter resolution.

Supporting engines return `guidance.authoredPlay.inspiration: true`, the boolean
`sheet.inspiration` and its saved explanation, including for incomplete builds.
Consumers enable editing only with that guidance. Existing requests that omit
the field remain supported in v4; older providers may reject a request containing
it. Consumers must retain saved reading and fail changes without silently
dropping the value. The Sheets schema includes the same optional DTO field;
its reviewed upgrade is documented by the [persistence owner](../../addon-dnd-character-sheets/docs/RULES_EDGE_CASES.md#inspiration-and-compatible-schema-upgrades).

## Species size

A species may declare `sizeOptions` as distinct canonical size labels.
The Engine returns one `kind: "size"` creation descriptor with stable ID
`species:<id>:size`, count one and source-owned options. Option `labelKey`
is an optional translatable UI key; absent keys leave authored labels intact.
The existing `build.choices` array owns the selection; there is no new saved
schema or automatic default.

`sheet.derived.size` and its explanation retain the chosen value and species
evidence. Fixed-size records use their canonical `size` directly. Unselected or
invalid size choices produce null; undeclared compound summaries omit the
projection field. Neither produces an inferred choice. Missing choices can save as incomplete builds; forged values/slots
use ordinary validation and stable repair issue IDs. Changing sources does
not mutate Engine inputs. The coordinator's existing explicit save/adoption
policy determines whether an already saved invalid selection is withdrawn.
This covers base species size, not temporary transformations or combat rules.

## Storage containers and membership

`play.containers` optionally stores ordered `{id,name}` organizational groups.
An inventory instance may declare `containerId` referencing one of those groups.
IDs must be distinct and nonempty; labels are nonblank and at most 120 characters.
The 500-container input limit is a payload bound, not physical carrying capacity.
Duplicate labels remain legal; identity is always the ID. There is no nesting,
source-item creation, weight calculation or additional equipment effect.

Membership is limited to carried/stored entries, including depleted quantities.
It never changes location, attunement allocation, notes, grants or quick-use
availability. Equipping requires explicitly clearing that item's membership.
Removing a container requires unassigning its members in the same input; dangling
references are blockers, never automatically deleted or redirected.

`guidance.authoredPlay.storage: true` advertises editing support.
`guidance.storage` supplies `maximumContainers` and `maximumNameLength`.
Saved `sheet.storage.containers` retains each ID, name and ordered `itemIds`,
including empty groups and depleted entries. Evaluation and play preserve authored
groups and membership; absent optional fields stay absent for older characters.
Invalid inputs remain intact for correction, including unfinished builds.

## Quick-use inventory references

`play.quickUse` is an optional ordered list of distinct owned inventory instance
IDs, bounded by the character input limit. Omitting it leaves existing characters
unchanged. Pins never duplicate item records, quantities, source references,
grant links or notes. Stored and depleted instances may remain pinned; missing
or duplicate references block saving without normalization.

`guidance.authoredPlay.quickUse: true` advertises support. Per-instance
`guidance.quickUse[id]` returns `canUse` and a stable reason (`empty`, `stored`,
or `build`). The saved `sheet.quickUse` rows and explanation retain order,
instance identity, name, quantity, location and availability for reading.
Availability describes the item; only live guidance authorizes an action.

`character-play` accepts `{operation:"consume-item", itemId}`. It uses one
positive-quantity carried/equipped instance and rejects extra fields. No item
effects, healing, rest replenishment or encounter resolution are implied. Using
the last unit retains the entry, pin and notes while clearing its attunement
and moving an equipped empty instance to carried, in the same detached result.
All ordinary validation still applies. Explicit item deletion must also remove
its pin in the same proposed input; an unpin alone never deletes inventory.
Consumers persist through their existing authenticated optimistic command and
exact-retry boundary.

## Equipment eligibility

`guidance.equipment` is keyed by inventory instance ID. Each entry exposes
`slot` (`armor`, `shield` or `worn`), `canEquip` and `canAttune`.
A rejected action includes `equipReason` or `attuneReason`: `empty`,
`source`, `mechanics`, `not-required`, `build`, `capacity`, `duplicate`
or `prerequisite`. These are stable translation codes, not instructions to
delete items. `sheet.equipment[id].slot` retains the same source-declared slot
for provider-free display; it does not retain permission to perform an action.

Armor types declare exclusive armor/shield occupancy independently of catalog
kind or record ID. Other worn items coexist. Equipping a replacement requires
an explicit client edit of the occupied slot; the Engine rejects conflicting
equipment and never moves stored items itself. Capacity includes all existing
attuned instances, including carried/stored items, and uses the final class,
item and authorized DM limit. Duplicate attunement has its own rejection.

Equipment and custom-item eligibility evaluate the requested state, including
equipped/attuned grant conditions. Attunement prerequisites are checked without
the candidate's own attunement effects or attunement-conditioned grants and
waivers. Independent active grants can qualify it. A lost prerequisite blocks
saving until explicitly repaired; evaluation never silently unattunes an item.
Unknown predicates still need an exact authorized waiver. This does not add
class/spellcaster predicate vocabulary, rest timing, distance or death tracking.

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
and commit the exact accepted result atomically. Engine output
never grants persistence authority to a browser.

The public `character.AcquisitionOwner` helper owns acquisition-key encoding.
`character.RemapGrantReferences` accepts an explicit old-to-new DM grant ID map
and returns detached inputs for a coordinator's reviewed import. It follows
nested acquisition owners in choices, spells, casting abilities, spent counters,
activations, roll resource references, item links, resource-capacity effects and
waivers. Renaming is simultaneous; empty/reused target IDs, reference collisions,
invalid encoding and excessive nesting are rejected. It does not rename grants,
stamp authority, recalculate values, reset counters or rewrite authored text.
The coordinator remains responsible for new grant identities and provenance.

Evidence identifies each contributing record, book, content hash, bounded
summary, mechanical facts, package ID, archive generation hash and content
revision. The sheet additionally retains the engine's package version/hash.
Missing sources cannot retroactively alter this saved evidence. Current rule
links remain subject to current source and viewer access. Statistic explanations
reference records read by the calculation. Learned/prepared spell records are
also retained for offline reading, without multiplying their references across
unrelated statistics; a spell actually read by a calculated grant remains a
calculation source. Ability increases, training, class spell statistics and resource
capacities/remaining uses link to their relevant granting sources, including
acquisition-owned grants, matching hit-die classes and shared/Pact slot owners.
The full evidence, hashes and calculation terms remain retained once; narrative
features do not multiply those per-statistic source lists. Typed item/DM effect
terms retain their own source or authorization identity.

### Passive source bonuses

Selected grant sources share `hpPerLevel`, `hpBonus`, `speedBonus` and
`senses`. Fixed HP is added once after recorded level gains and Constitution;
speed sums source bonuses once before armor penalties and typed item/DM
effects. Sense grants use the greatest range before explicit adjustments.
A recalculation is detached and never heals or otherwise edits authored play.

`grants.acBonuses` adds to eligible AC formulas and shields. Its optional
`requires.armorTypes` condition tests equipped body armor, using the item's
declared reference kind and armor type. Shields alone do not satisfy it.
Unknown or malformed conditions remain inactive. The saved AC explanation
retains applied/inactive terms and source references; maximum HP retains
fixed-grant terms. See the provider's [field shapes](../../addon-dnd-2024-compendium/data/SCHEMA.md).
Neither the Engine nor the sheet identifies these rules by feat or book ID.

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

### Acquisition-owned spell grants

Repeatable feat spell selections, casting abilities, resource pools, bonus slots and activations use the same stable acquisition owner as Builder choices. A free-cast counter belongs to its source and, for a single selected spell, its granting choice. Replacing that spell does not refresh its spent allowance. Cast commands can spend only that grant's free counter or an eligible spell slot.

Returned descriptors provide `source.acquisition` and a `legacyKey` when an older unscoped key differs. Detached character evaluation moves an old key only to one unoccupied owner; ambiguous choices, abilities, activations and spent counters remain authored input for explicit assignment. Play commands use this normalized input, so an old spent allowance cannot refresh during its first command. Reading never commits a migration.

Generic `choicePackages` resolve against each acquisition. Source `originFeatChoices` presets constrain the corresponding origin-granted feat choice; the Engine applies those defaults without rewriting the source record. Unknown conditional-repeat rules remain closed.

Class skill choices use starting proficiencies only for the initial class.
Every later class uses its declared multiclass skill pool; an absent pool
means no additional starting skills. Changing the ordered initial class
recomputes those choices without changing the source catalog.

### Multiclass progression and item prerequisites

Starting skills, equipment training and weapon-attack proficiency use the same
class-order decision. Later classes receive only `multiclassProficiencies`;
an absent reduced pool gives no starting training. Spell progression comes
from the record owning `spellcasting`. A subclass's ordinary feature table
cannot replace its class's spell limits. Pact Magic retains a separate pool
and does not count as a second Spellcasting class: one Spellcasting class
keeps its declared slot table; multiple use profile-owned fractions and slots.
Spell preparation limits always use each class's own level.

Item `attunementPrerequisites` accepts generic `classes` minimum-level maps
and `spellcaster: true`, including inside `all`/`any`. Intrinsic class spell
capacity, trait/feat cantrips and granted free spells qualify independently of
spent uses. Item-granted spells and unusable prepared-only grants do not.
Unknown predicates remain blocked for explicit DM adjudication. Losing a
prerequisite never silently unattunes an authored item; repair must be explicit.
These checks do not replace the separate item-mechanics authority gate.
