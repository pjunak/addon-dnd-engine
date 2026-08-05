# D&D Rules Engine Addon

Headless, reusable D&D rules computation for
[ttrpg-codex](https://github.com/pjunak/ttrpg-codex).

Addon id: `dnd-engine`. It is an independently installable API-v2 addon with no
UI permissions and no character storage.

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

- consumes zero or one `dnd5e.rules-data` `^2.0.0` service;
- provides `dnd5e.rules-engine` `2.0.0`.

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

## Compatibility transition

Existing installations should roll out the separation in this order:

1. Install `dnd-engine`. Existing sheet releases ignore it safely.
2. Update the rules-data addon to contract v2. A v2 provider publishes one
   complete rules profile; there is no engine-owned base to inherit.
3. Update `dnd-sheets`. It discovers the engine through
   `dnd5e.rules-engine`; no compendium or engine addon ID is encoded in the
   sheet.

Reversing the last two steps can temporarily leave the Builder without rules
automation, but stored and materialized sheet fields remain usable. A provider
declares the rules-data service exclusive so the host requires uninstalling an
existing revision before another one can be installed.

## Development

Node.js 26 is required. This project uses browser-native ES modules and has no
build step or runtime package dependencies.

Run all tests from this repository with relative paths:

```text
node --test tests/*.mjs
```

Install the working tree from a sibling `ttrpg-codex` checkout:

```text
node scripts/dev-install-addon.cjs ../addon-dnd-engine
```

See [`contract/README.md`](contract/README.md) for the public service contracts,
[`rules/README.md`](rules/README.md) for computation semantics, and
[`AGENTS.md`](AGENTS.md) for repository policy.

## License

This project is licensed under the [MIT License](LICENSE).
