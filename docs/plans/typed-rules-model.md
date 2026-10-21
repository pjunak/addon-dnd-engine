# Plan: typed rules model (rules-engine 6)

Temporary plan for the quality pass's "Typed rules model" stage. When the
Engine work lands, `contract/README.md` describes the result and this file is
deleted.

## Goal and scope

No public output is `map[string]any`, and no closed set is a string switch.
Computed results stay identical to Engine 5.4.0 except for the deliberate
changes listed below; the regression vectors and existing tests are the check.

In scope:

- Typed public outputs in the `character` package: `Sheet`, `Plan`,
  `Guidance` and `SpellOptions`, built directly by the evaluation instead of
  as maps.
- A Go type for each closed set, with one dispatcher (a switch or a spec
  table) checked by the `exhaustive` linter (`-check=switch,map`).
- A typed play command (`PlayCommand`) instead of the `change` map.
- Generators that emit enums, tagged unions and consistent nullability.
- Bad rules data reported as issues; no string-to-number coercion.
- The HP clamp in evaluation; the unused `derive` method removed.

Follow-ups, outside this stage because they change no public contract:

- Internal Builder decisions are still a map built from `character.Inputs`.
- Rules records are still read field by field from decoded JSON. Grants are
  validated against the typed vocabulary, but read through the map.

## Typed sets

Each set is a named string type in `character` (public) or `internal/rules`
(record vocabulary), with constants and an `EnumValues()` method that the
generators read. Values are today's strings, so JSON does not change.

| Type | Values | Dispatcher |
| --- | --- | --- |
| `PlayOperation` | `rest`, `toggle-feature`, `select-spell`, `cast-spell`, `select-grant-spell`, `select-casting-ability`, `cast-granted-spell`, `cast-ritual`, `copy-spell`, `swap-spell`, `consume-item`, `spend-hit-die`, `set-hp`, `heal`, `damage`, `set-temporary-hp`, `replace-class-choice`, `set-hand`, `set-grip`, `keep-concentration`, `end-concentration` | field table for decoding; one switch in `ApplyCharacterPlay` |
| `EffectTarget` | `abilityScore`, `abilityCap`, `savingThrow`, `armorClass`, `initiative`, `speed`, `maxHp`, `attunementLimit`, `sense`, `resourceMax`, `proficiency` | spec table: key rule and explanation path |
| `EffectMode` | `set`, `add`, `minimum`, `maximum` | switch in `effectNumber` |
| `GrantCondition` | `always`, `equipped`, `attuned` | switch in `characterGrantActive` |
| `SourceType` | grant sources `class`, `subclass`, `feature`, `species`, `background`, `feat`, `item`; sheet-only sources `spellcasting`, `pactMagic`, `armor`, `condition`, `spell`, `asi`, `ability` | one `Source` struct for every `source` |
| `ChoiceKind` | `skills`, `expertise`, `skillExpertise`, `skillUpgrade`, `tools`, `proficiencies`, `weaponMastery`, `languages`, `savingThrows`, `weapons`, `armor`, `resistances`, `immunities`, `feat`, `enumerated`, `asiMode`, `abilityBudget`, `size` | spec table: heading, resolved proficiency, default option pool |
| `ChoiceType` (record `choices[].type`) | the `grants.choices[].type` vocabulary | table mapping to `ChoiceKind` |
| Calculation operations | removed with `derive` | — |

Smaller sets typed on the way: `IssueSeverity`, `ResourceKind`, `RestKind`,
recharge `on`, `RollKind`, `RollMode`, activation modifier targets, hand grip,
spell selection lists, caster types, condition status and the sheet status.
Input fields get enums only where evaluation already blocks other values, so
no saved character becomes invalid.

## Typed outputs

`character.Result` keeps its fields; four become typed:

- `Sheet`: the projection. `status` is `calculated` or `needs-choices`. The
  calculated statistics sit in an embedded `*Calculation` whose fields stay at
  today's paths and are absent while choices are missing, so explanation
  paths and JSON do not move. Play state (inspiration, hands, quick use,
  storage, conditions, equipment, carrying, concentration) is always present.
  Every `source` is the shared `Source` type.
- `Plan`: base scores, classes, point-buy policy and `Choice` descriptors (one
  struct with optional kind-specific fields; `kind` is `ChoiceKind`).
- `Guidance`: sections, choice entries, class steps, equipment, hands, quick
  use, conditions, storage, save issues and replacements.
- `SpellOptions`: per-class options, pending choices and granted spells (the
  sheet's types plus their options), casting-ability choices and slots.

Mixed JSON values get small types with their own JSON form: movement value
(feet or `"speed"`) and recharge amount (`full`, `halfLevel`, a number or
`{abilityMod}`). `Evidence.facts` becomes an opaque JSON object
(`json.RawMessage`), the provider's record excerpt. Explanation values stay
display values (any JSON).

Deliberate changes:

1. `derive` is removed from the service.
2. `change` is a typed `PlayCommand`; the schema is a tagged union.
3. Calculated sheets carry `status: "calculated"`.
4. Hydration emits record identities for `class`, `species` and `background`
   (what the public sheet already had), not the full records.
5. `labelArgs` are strings.
6. `derived.size` is absent until known (it was null or absent).
7. Requirement copies in `ac.bonuses` and `speedBonuses` keep only the
   supported fields.
8. A lowered maximum HP clamps `inputs.play.hp` in every calculated result;
   Sheets stops clamping.
9. A record whose `grants` break the vocabulary gives a
   `rules-data:<kind>:<id>` warning, and its grants are ignored. Numeric
   strings in records are no longer read as numbers.
10. Arrays and objects are never null in any output (see nullability).

## Versions

- Service `dnd5e.rules-engine` 5.0.0 → 6.0.0.
- Character contract `rules-character.v2` → `v3`; response
  `rules-character-response.v2` → `v3`.
- Package `dnd-engine` 5.4.0 → 6.0.0.
- `dnd5e.rules-data` consumption is unchanged (^3.0.0).

## Generators

One reflection walker in `character` serves both generators, so the schema
and TypeScript always agree. Sheets' TypeScript generator moves into it
(`character.TypeScript`), and Sheets calls it for its own types too.

- **Enums**: a type with `EnumValues()` emits `{"enum": [...]}` and a named
  TypeScript union of string literals.
- **Tagged unions**: a type with `UnionVariants()` (tag field and fields per
  variant) emits a `oneOf` of closed objects with a `const` tag, and a
  TypeScript union of object types. `PlayCommand` uses it.
- **Nullability**: `omitempty` (or an embedded pointer) means optional, never
  null. A pointer without `omitempty` is required and nullable (`X | null`).
  Slices and maps are never null; `character.FillEmpty` replaces nil slices and
  maps before a result leaves the Engine (and before Sheets stores or returns
  state), and tests check outputs against the generated schema rules.
- **Custom JSON types** implement `JSONSchema()` and `TypeScriptType()`.

## Other repositories

- **Sheets** reads typed results, uses `PlayCommand`, drops its HP clamp,
  bumps the stored schema and upgrades saved state and old exports by
  recomputing the projection from saved inputs.
- **Compendium** enforces the Engine's generated grants vocabulary
  (`contracts/grants.schema.json`) in `contracts/rule-record.schema.json`.
- **Homebrew** takes the updated record schema copy and Engine requirement.
- **Host** installed tests follow the new versions; `derive` checks go.
