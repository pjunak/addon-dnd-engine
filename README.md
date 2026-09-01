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

`addon-dnd-character-sheets` will retain its stable `dnd-sheets` data namespace, editing model,
sheet shell, Compact and Classic built-in renderers, and renderer selection.
Compact will be the default for characters without a saved per-browser choice.

## Engine service

The v3 service exposes explicit, schema-validated worker calls:

- `context` reports provider availability and exact provider/ruleset identity;
- `get-record` and `query-records` expose provider-neutral rule records;
- `derive` performs deterministic universal or ruleset-backed arithmetic.

Every ruleset-backed response carries the provider package generation, content
revision, ruleset ID, ruleset version, and edition. The worker can therefore
reject or expose stale context rather than silently mixing revisions.

## Development

Go 1.26 is required. The repository builds static native workers for Windows
amd64, Linux amd64, and Linux arm64.

Run the Go checks from this repository:

```text
go test ./...
go vet ./...
go test -race ./internal/rules ./internal/provider ./internal/engine
```

Build the worker binaries and deterministic install archive:

```text
go run ./cmd/build-package
```

The generated archive is written to `dist/dnd-engine-3.0.0.zip`. Platform
binaries under `worker/` are committed because host source installs download
the repository rather than compiling add-on source.

Install the working tree from a sibling `ttrpg-codex` checkout:

```text
node scripts/dev-install-addon.cjs ../addon-dnd-engine
```

See [`contract/README.md`](contract/README.md) for the public service contracts,
[`rules/README.md`](rules/README.md) for computation semantics, and
[`AGENTS.md`](AGENTS.md) for repository policy.

## License

This project is licensed under the [MIT License](LICENSE).
