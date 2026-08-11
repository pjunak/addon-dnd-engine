# AGENTS.md — addon-dnd-engine

This repository contains the headless `dnd-engine` addon for the sibling
`ttrpg-codex` host. Independently developed character sheets can discover its
`dnd5e.rules-engine` service without installing the official sheet UI. The
manifest ID and service identities are permanent compatibility contracts.

## Read before editing

1. [`README.md`](README.md) for the repository purpose and boundaries.
2. `../ttrpg-codex/examples/addons/AGENTS.md` for the host addon contract.
3. `../ttrpg-codex/docs/reference/addons.md` before changing manifests,
   service discovery, lifecycle, permissions, or installation behavior.
4. [`contract/README.md`](contract/README.md) before changing public services,
   ruleset validation, provider metadata, or reference resolution.
5. [`rules/README.md`](rules/README.md) before changing computation semantics.
6. `../addon-dnd-2024-compendium/data/SCHEMA.md` when a provider record field changes.

Do not duplicate the host authoring guide or compendium record catalog here.
Link to their authoritative documents.

## Intended architecture

```text
addon.json          API-v2 manifest and service declarations
entry.js            composition root; service binding and lifecycle only
contract/           public engine/provider schemas and conformance fixtures
rules/              deterministic host-free computation modules
tests/              unit, contract, provider, lifecycle, and smoke tests
```

These ownership rules are mandatory:

- The engine computes. It owns no pages, fragments, CSS, catalogs, or sheet
  presentation.
- The engine owns no character persistence. Callers pass detached input data
  and receive detached results.
- The engine consumes a compatible rules-data service through the host's
  generic service registry. Never probe known addon IDs or sourcebook IDs.
- Provider identity, contract version, ruleset identity/version, and content
  revision remain explicit across the service boundary.
- A ruleset is always complete. The engine owns no edition base and rejects
  inheritance or implicit profile defaults.
- Pure rule functions stay independent of the DOM, browser globals, CodexHost,
  storage, network access, and addon lifecycle.
- Provider records supply rule facts and provenance. Engine code interprets
  generic documented fields and never branches on a book or product ID.
- Public APIs are small, versioned, immutable where practical, and fail closed
  on incompatible input. Internal helpers are not exported for convenience.
- Hydration remains deterministic and returns structured warnings rather than
  throwing for ordinary incomplete character choices.
- Do not add combat resolution or encounter automation; those are separate
  addon concerns.

## Compatibility and migration

- The existing `dnd-sheets` manifest ID and
  `character.addonData["dnd-sheets"]` remain owned by the sheets addon. This
  repository must not migrate or write that namespace.
- Preserve engine behavior with repository-owned regression vectors. Change
  semantics only in explicit, separately tested commits.
- Installing, disabling, updating, or replacing a provider must participate in
  host lifecycle ordering. A stale provider instance must never survive a
  content-revision change.

## Code quality

- Use browser-native named ES-module exports. There is no bundler or
  transpiler.
- Keep functions focused and pass dependencies explicitly.
- Prefer immutable inputs/results and pure helpers. Clone at the contract
  boundary when caller mutation could leak across consumers.
- Write self-documenting code. Add comments only for non-obvious invariants,
  constraints, or safety decisions.
- Avoid hidden fallbacks, global registries outside the host facade, and
  catch-all compatibility branches.
- Add regression tests for every bug and contract tests for every public
  surface. A provider fixture must be synthetic and legally redistributable;
  do not copy compendium content into this repository.

## Working loop

Use Node.js 26 and PowerShell on Windows. Once executable code exists, the
complete repository test command is:

```text
node --test tests/*.mjs
```

Use relative test paths on Windows. From the host repository, install the
working tree with:

```text
node scripts/dev-install-addon.cjs ../addon-dnd-engine
```

Source edits are not visible in the running app until dev-reinstall. Keep test
metadata in `addon.json` synchronized with the real test inventory. Add the
ordinary test workflow in the same commit that introduces executable source
and meaningful tests; do not create a permanently green no-test workflow.

The only durable suite backlog is
`../ttrpg-codex/docs/BACKLOG.md`. Temporary cross-repository implementation
plans belong only in the host repository's ignored `docs/plans/` directory and
must be deleted when the work closes. Do not create repository-local TODO or
roadmap files.

The global Codex instructions govern task commits. Keep public-contract and
computation changes separate when they are independently reviewable. Never
push commits or tags unless the maintainer explicitly requests it.
