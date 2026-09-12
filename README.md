# D&D Rules Engine

The headless `dnd-engine` Add-on API v3 package evaluates character decisions
and returns calculated values, choices, constraints and source explanations.
It provides `dnd5e.rules-engine` **4.0.0** and consumes all compatible
`dnd5e.rules-data` **^3.0.0** providers through the host service broker.

The engine has no character storage or UI. The sheets package owns its retained
decisions and accepted results. Each evaluation uses the instance's complete
rules profile and enabled books. Missing providers make character evaluation
unavailable; universal `derive` operations remain usable.

## Public operations

- `context`: availability and current provider/rules identity.
- `get-record`, `query-records`: provider-neutral structured records.
- `derive`: named universal or profile-backed arithmetic.
- `evaluate-character`: detached character inputs to a complete result,
  including unresolved choices and blockers.
- `character-play`: one bounded play command followed by the same evaluation.

See [the service contract](contract/README.md), [calculation ownership](rules/README.md)
and [generated schemas](contracts/rules-engine.service.json). Version 4 replaces
the previous public Builder/hydration/play handlers. Shared arithmetic helpers remain internal. Fixture-only play/reconciliation
adapters live in test files and are excluded from the production package.

## Development and packaging

Use Go from [go.mod](go.mod). Its local SDK replacement expects a compatible
`ttrpg-codex` checkout beside this repository.

```text
go test ./...
go vet ./...
go test -race ./internal/rules ./internal/provider ./internal/engine
go run ./cmd/build-package
```

The build generates character schemas from the public Go model and packages
committed Windows amd64, Linux amd64 and Linux arm64 workers. Inspect the ZIP
from the host with `go run ./cmd/codex-addon-inspect ../addon-dnd-engine/dist/dnd-engine-4.0.0.zip`.
Installation uses stage, permission review, approval and activation.

The host's installed rules suite evaluates every packaged class at levels 1, 5
and 20, changes content and source policy, and exercises provider loss/recovery.
The installed character suite checks the coordinating worker and sheet UI.
These are separate from the pure arithmetic regression vectors.
