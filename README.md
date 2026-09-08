# D&D Rules Engine Addon

Headless, reusable D&D rules computation for
[ttrpg-codex](https://github.com/pjunak/ttrpg-codex).

Addon id: `dnd-engine`. It is an independently installable API-v3 native-worker
addon with no UI permissions and no character storage.

## Purpose

This addon turns structured rules data and a detached character decision
model into deterministic computed results. It exists separately from the
official sheets so multiple sheet implementations can share the engine and a
compatible replacement engine can be selected without changing those sheets.

```text
rules-data provider (official or third-party)
                    |
                    v
          headless D&D engine
                    |
          +---------+---------+
          |                   |
          v                   v
 official character sheet   custom sheet
```

The official `addon-dnd-2024-compendium` is one rules-data provider. Provider and engine
selection use generic versioned CodexHost services rather than recognized addon
IDs:

- consumes zero or one `dnd5e.rules-data` `^3.0.0` service;
- provides `dnd5e.rules-engine` `3.0.0` through the host service broker.

With no provider, universal arithmetic remains available and provider-dependent
hydration reports its unavailable state. Multiple compatible providers are
resolved explicitly by the host Add-on Manager.

## Boundaries

The engine will own:

- Pure D&D derivation functions.
- The versioned engine API.
- The consumer-side rules-data provider contract and conformance fixtures.
- Complete profile validation and normalized Builder choice interpretation.

The engine will not own:

- Character storage or migrations.
- Sheet layout, renderers, CSS, routes, fragments, or browser preferences.
- Compendium records, sourcebook provenance, or browsing UI.
- Edition profiles, advancement tables, origin policy, or native fallback rules.
- Combat encounter automation.

`addon-dnd-character-sheets` retains its stable `dnd-sheets` data namespace and
owns sheet presentation. Acceptance of the former Compact/Classic layouts and
renderer selection remains an open product gate in the
[suite backlog](../ttrpg-codex/docs/BACKLOG.md), not an engine guarantee.

## Engine service

The v3 service exposes explicit, schema-validated worker calls:

- `context` reports provider availability and exact provider/ruleset identity;
- `get-record` and `query-records` expose provider-neutral rule records;
- `derive` performs deterministic universal or ruleset-backed arithmetic;
- `hydrate` turns detached character decisions into computed sheet state while
  preserving a provider-free universal fallback;
- `builder-plan`, `apply-builder-choice`, and `reconcile-builder-decisions`
  keep edition-specific creation choices explicit and reviewable;
- `spell-options` describes eligible spell/grant choices, casting slots,
  rituals and copying costs without changing authored state;
- `apply-play-change` returns detached rest, hit-die, activation, class/grant
  spell, casting-ability, copying or swap changes plus hydration and refreshed
  spell options from one provider evaluation. It never persists data.

Every ruleset-backed response carries the provider package generation, content
revision, ruleset ID, ruleset version, and edition. The worker can therefore
reject or expose stale context rather than silently mixing revisions.

## Development

Go 1.27.1 is required, matching the host SDK module. The repository builds static native workers for Windows
amd64, Linux amd64, and Linux arm64.

Run the Go checks from this repository:

```text
go test ./...
go vet ./...
go test -race ./internal/rules ./internal/provider ./internal/engine
```

The Go suite includes 144 complete hydration and Builder comparisons against
the preserved v1 engine, using repository-owned synthetic records. Expected
results retain the original v1 output; the reviewed class weapon-proficiency
summary correction is explicit. To regenerate with Node.js 26 and the preserved
Git history available locally, run `node tools/generate-v1-vectors.mjs`.
See [`rules/README.md`](rules/README.md) for coverage and provenance.

Build the worker binaries and deterministic install archive:

```text
go run ./cmd/build-package
```

The generated archive is written to `dist/dnd-engine-3.0.0.zip`. Platform
binaries under `worker/` are committed so deterministic packages contain every
supported deployment target without compiling source in production.

Inspect the release candidate from a sibling host checkout:

```text
go run ./cmd/codex-addon-inspect ../addon-dnd-engine/dist/dnd-engine-3.0.0.zip
```

Installation uses the host's reviewed package upload flow during supervised
integration testing.

See [`contract/README.md`](contract/README.md) for the public service contracts,
[`rules/README.md`](rules/README.md) for computation semantics, and
[`AGENTS.md`](AGENTS.md) for repository policy.

## License

This project is licensed under the [MIT License](LICENSE).
