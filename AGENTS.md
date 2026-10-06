# D&D Rules Engine add-on

The headless `dnd-engine` add-on for TTRPG Codex. A native Go worker that
provides `dnd5e.rules-engine` v4 and consumes any compatible `dnd5e.rules-data`
v3 provider. It has no UI and stores nothing. The add-on ID and service
identities are permanent.

This is a personal project: keep rules correct and code clear; old formats and
extra hardening are low priority.

## Commands

Go only (version in `go.mod`); no Node toolchain.

```text
go run ./tools/check.go          # gofmt, vet, staticcheck, all tests, race tests
go run ./tools/check.go fast     # static checks only
go run ./tools/check.go format   # apply gofmt
go run ./cmd/build-package       # dist/dnd-engine-<version>.zip
go tool -modfile=go.tools.mod codex-addon-inspect dist/dnd-engine-<version>.zip
```

The repository builds from a plain clone: the host's worker SDK is a normal Go
module requirement. To work against a local host change, use an uncommitted
`go work init . ../ttrpg-codex`. Regenerate the public character schemas with
`go run ./cmd/character-contract` when the `character` model changes.

## Layout

```text
cmd/worker/         worker composition only
cmd/build-package/  reproducible package build (Windows amd64, Linux amd64/arm64)
character/          public character model, shared with Character Sheets
contracts/          public JSON Schemas of the service
internal/engine/    service boundary
internal/provider/  rules-data client
internal/rules/     pure, deterministic rules computation
```

## Rules

- The engine computes; it owns no pages, persistence or character state.
  Callers pass detached inputs and receive detached results.
- Find rules data through the host service broker by contract. Never branch on
  an add-on, book or product ID; interpret the documented record fields.
- A ruleset is complete and explicit: no edition defaults or inheritance.
- Keep `internal/rules` free of the worker protocol, host services, storage and
  network. Incomplete choices produce structured warnings, not errors.
- Keep public APIs small and versioned; fail closed on incompatible input.
- No combat or encounter automation; that belongs in a separate add-on.
- Add a regression test for every bug. Test fixtures are synthetic; never copy
  compendium content here.

## Read more

[README.md](README.md) (purpose and install), [contract/README.md](contract/README.md)
(service contract), [rules/README.md](rules/README.md) (calculation ownership),
and the host's [add-on guide](https://github.com/pjunak/ttrpg-codex/blob/main/examples/addons/AUTHORING.md).
Tasks for all repositories live in the host's `docs/BACKLOG.md`.

Successful `main` builds publish the inspected ZIP as a GitHub release; DMs
install it through Settings → Add-ons. Ask before pushing.
