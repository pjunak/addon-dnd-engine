# D&D Rules Engine Addon

Headless, reusable D&D rules computation for
[ttrpg-codex](https://github.com/pjunak/ttrpg-codex).

> **Status:** repository scaffold. The engine still lives in
> `dnd-character-sheets/rules/`; this repository is not an installable addon
> until the versioned service contract, implementation, manifest, and tests are
> extracted together.

## Purpose

This addon will turn structured rules data and a detached character decision
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

The current `dnd55e-compendium` will be one rules-data provider. Provider
selection will use a generic, versioned CodexHost service contract rather than
a list of recognized addon IDs.

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

Once the extraction introduces executable source, run all tests from this
repository with relative paths:

```text
node --test tests/*.mjs
```

Install the working tree from a sibling `ttrpg-codex` checkout:

```text
node scripts/dev-install-addon.cjs ../addon-dnd-engine
```

See [`AGENTS.md`](AGENTS.md) for the repository contract. The detailed
cross-repository implementation plan is intentionally kept under the host's
gitignored `docs/plans/` directory rather than duplicated here.

## License

This project is licensed under the [MIT License](LICENSE).
