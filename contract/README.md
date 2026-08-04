# D&D service contracts

This repository is the consumer-side compatibility authority for two CodexHost
services:

- `dnd5e.rules-data` `1.0.0`: structured rule records and one explicit,
  validated ruleset from a replaceable provider.
- `dnd5e.rules-engine` `1.0.0`: headless deterministic computation and the
  provider-neutral list/get/reference surface used by sheet builders.

Provider selection is handled by CodexHost. Consumers never probe addon IDs.
The selected handle supplies provider package and content revision metadata;
`getContextIdentity()` combines that metadata with the ruleset identity.

## Rules-data v1

The provider API has `apiVersion: 1` and implements the list/get functions in
[`rules-data.js`](rules-data.js), plus `getRuleset()`. A ruleset has stable
`rulesetId`, positive `rulesetVersion`, and `edition`. It must either contain
all required constants and capability flags or explicitly declare
`extends: "dnd-2024"`. Missing fields never imply cross-edition inheritance.
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

## Engine v1

The engine API exposes availability and context identity, `hydrate`, granular
`derive` helpers, and delegated list/get/reference methods. The derivation
surface includes ability/proficiency math plus ruleset-aware hit-die averages,
scroll-copy cost, point-buy cost/total, feat ASI options/caps, multiclass slots,
initiative, hit points, Armor Class, and save DC. Sheet consumers delegate
these facts instead of carrying edition tables.

The engine owns no Store, character namespace, UI, routes, or persistence.
Without rules data it exposes provider-neutral arithmetic and reports
rules-data-dependent hydration as unavailable instead of selecting a hidden
addon by id.
