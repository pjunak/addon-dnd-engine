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

Standalone CI fetches the exact host commit in
[host-sdk-revision.txt](host-sdk-revision.txt) before checking or packaging.
Update that pin in a new add-on commit to ship an SDK fix; rerunning an old
commit must not silently compile a newer SDK or replace its published ZIP.
`go run ./tools/check.go dependency-ref host-sdk-revision.txt` validates the pin
without requiring sibling modules. Local builds use the adjacent checkout;
the host's compatibility suite deliberately builds against its candidate SDK
and records the actual source commits and package hashes separately.

```text
go run ./tools/check.go
go run ./cmd/build-package
```

The check runner uses gofmt, vet and pinned Staticcheck, then runs all tests and
the rules/provider/engine race tests. Use `go run ./tools/check.go fast` for the
short edit loop, `format` to apply gofmt, `workflows` for actionlint, and `vuln`
for govulncheck against the current vulnerability database. The race detector
requires a supported C compiler. CI runs the analysis gates before publication
and checks workflows/vulnerabilities weekly. Tool versions live separately in
`go.tools.mod` and `go.tools.sum`; no npm toolchain is needed for this engine.

Generate versioned character schemas from the public Go model with
`go run ./cmd/character-contract` when that model changes. The package command
rebuilds Windows amd64, Linux amd64 and Linux arm64 workers from source;
`worker/` and `dist/` are ignored build output, never committed. CI requires
builds to leave tracked source unchanged. Inspect the ZIP
from the host with `go run ./cmd/codex-addon-inspect ../addon-dnd-engine/dist/dnd-engine-4.0.0.zip`.
Installation uses stage, permission review, approval and activation.

The host's installed rules suite evaluates every packaged class at levels 1, 5
and 20, changes content and source policy, and exercises provider loss/recovery.
The installed character suite checks the coordinating worker and sheet UI.
These are separate from the pure arithmetic regression vectors.


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
