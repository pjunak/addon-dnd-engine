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

[`synthetic-provider.mjs`](synthetic-provider.mjs) is the redistributable
conformance fixture. Provider repositories should run equivalent validation
against their real API without copying production content here.

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

The engine owns no Store, character namespace, UI, routes, or persistence.
Without rules data it exposes provider-neutral arithmetic and reports
rules-data-dependent work as unavailable instead of selecting a hidden addon or
applying a bundled edition profile.
