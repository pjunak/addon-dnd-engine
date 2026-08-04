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

The current `dnd55e-compendium` is one rules-data provider. Provider and engine
selection use generic versioned CodexHost services rather than recognized addon
IDs:

- consumes zero or one `dnd5e.rules-data` `^1.0.0` service;
- provides `dnd5e.rules-engine` `1.0.0`.

With no provider, universal arithmetic remains available and provider-dependent
hydration reports its unavailable state. Multiple compatible providers are
resolved explicitly by the host Add-on Manager.

## Boundaries

The engine will own:

- Pure D&D derivation functions.
- The versioned engine API.
- The consumer-side rules-data provider contract and conformance fixtures.
- Explicit ruleset identity, compatibility, and validation.

The engine will not own:

- Character storage or migrations.
- Sheet layout, renderers, CSS, routes, fragments, or browser preferences.
- Compendium records, sourcebook provenance, or browsing UI.
- Combat encounter automation.

`dnd-character-sheets` will retain its stable data namespace, editing model,
sheet shell, Compact and Classic built-in renderers, and renderer selection.
Compact will be the default for characters without a saved per-browser choice.

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
