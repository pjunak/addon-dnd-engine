# D&D Rules Engine

The headless `dnd-engine` Add-on API v3 package evaluates character decisions
and returns calculated values, choices, constraints and source explanations.
It provides `dnd5e.rules-engine` **4.0.0** and consumes all compatible
`dnd5e.rules-data` **^3.0.0** providers through the host service broker.

The engine has no character storage or UI. The sheets package owns its retained
decisions and accepted results. Each evaluation uses the instance's complete
rules profile and enabled books. Missing providers make character evaluation
unavailable; `ability-modifier`, `clamp-hp` and `feat-asi-from` remain usable.
It requires a ruleset of version 4 or later (Compendium 3.3.0).

## Public operations

- `context`: availability and current provider/rules identity.
- `get-record`, `query-records`: provider-neutral structured records.
- `derive`: named arithmetic; formula operations read the rules profile and
  return its identity.
- `evaluate-character`: detached character inputs to a complete result,
  including unresolved choices and blockers.
- `character-play`: one bounded play command followed by the same evaluation.

See [the service contract](contract/README.md), [calculation ownership](rules/README.md)
and [generated schemas](contracts/rules-engine.service.json). Version 4 replaces
the previous public Builder/hydration/play handlers. Shared arithmetic helpers remain internal. Fixture-only play/reconciliation
adapters live in test files and are excluded from the production package.

## Development and packaging

Use Go from [go.mod](go.mod). The repository builds from a plain clone; the
host's worker SDK and package inspector are ordinary Go module requirements.

```text
go run ./tools/check.go
go run ./cmd/build-package
go tool -modfile=go.tools.mod codex-addon-inspect dist/dnd-engine-4.0.0.zip
```

The check runner uses gofmt, vet and pinned Staticcheck, then runs all tests and
the rules/provider/engine race tests (the race detector needs a C compiler).
Use `go run ./tools/check.go fast` while editing, `format` to apply gofmt,
`workflows` for actionlint and `vuln` for govulncheck. Analysis tools are pinned
in `go.tools.mod`.

The package command rebuilds Windows amd64, Linux amd64 and Linux arm64 workers
from source into the ignored `worker/` and `dist/` folders. Regenerate the
versioned character schemas with `go run ./cmd/character-contract` when the
public `character` model changes.

To develop against an unreleased host change, run `go work init . ../ttrpg-codex`
(the `go.work` file is ignored), then require the pushed host commit with
`go get github.com/pjunak/ttrpg-codex@<commit>`.

## Install and update from tested commits

Successful main builds publish the inspected ZIP to a permanent
[commit release](https://github.com/pjunak/addon-dnd-engine/releases). Each release identifies
the source commit even when the package version is unchanged. CI uses GitHub's
automatic repository token; it does not deploy to anyone's server.

In your website, open **Settings → Add-ons → Add add-on → GitHub**, enter
`pjunak/addon-dnd-engine` and use **Latest published package**. For an installed ZIP, use
**Update source** to link the same repository. **Check for updates** offers the
latest tested package; **Download and review** leads to explicit permission and
compatibility review before **Approve and activate**. Publishing never forces
an update on an installation.

Public release downloads do not require a GitHub token.

Existing Actions-build sources remain supported, but their artifacts expire.
Switch an existing source to **Latest published package** to use durable releases.
