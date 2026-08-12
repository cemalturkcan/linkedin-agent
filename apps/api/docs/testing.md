# Testing and quality gates

This service mirrors `dating-platform/apps/api`. The layers below are the reference's layers; the
only deviation is the store, which is SQLite in a temporary directory rather than a PostgreSQL
container, so an integration test here costs milliseconds and needs no image.

## Test layers

### Unit

Unit tests are isolated and fast. They use no store, no network, no wall-clock sleep and no
uncontrolled mutable process-global state. Inject `clock.Clock` and collaborators through the
existing package seams. Config parsing tests may use scoped `t.Setenv` and must not run in parallel.

```sh
make test-unit
```

`test-unit` runs the whole suite under `-short`. Anything that opens the store must call
`testutil.RequireIntegration` before it allocates anything, so this target stays isolated.
`testutil.NewStore` and `testutil.OpenStore` already enforce it.

The pure decision code is unit tested at this layer: settings normalisation, resume folder scanning,
derived profile shaping, the title term matcher, and the planner's query compilation. None of them
touch the store, so none of them are skipped under `-short`.

### Integration

Integration tests verify SQL, transactions, the schema and persistence against a real SQLite file
through `modernc.org/sqlite`. Do not replace the store with a SQL mock. Use a bounded context for
setup and for every operation.

```sh
make test-integration
```

Every test that needs a store calls `testutil.NewStore(t)`, which opens a fresh database under
`t.TempDir()` and closes it through `t.Cleanup`. Each test therefore owns a private, fully created
schema and no test can observe another's rows. There is no shared container, no template database
and no allocation lock: a fresh file is cheaper than any of them.

The schema is recreated whenever `PRAGMA user_version` does not match, and the store test proves
that a version mismatch drops what was there. That is the whole migration story before release, and
it is deliberate: the local store is disposable state that a round refills.

HTTP tests build the composition root through `Build`, drive it with `Server.Test`, and assert the
exact status, the envelope and the stable error text.

### Race

Concurrency-sensitive code must pass the complete suite with the race detector. The event hub, the
trace recorder's delta buffer and the plugin's quiet timer are the code this layer exists for.

```sh
make test-race
```

## Live model tests

Tests that call the real model are gated behind `LINKEDIN_AGENT_LIVE=1` and skip by default, so they
never run inside `make verify`. They bind a loopback port at or above 8861 and keep their data
directory under `/tmp`. They exist to prove behavior against the running system and the real model,
and their output is quoted in the handoff rather than asserted on.

Nobody in this repository can call LinkedIn: only the user's browser holds the session. Build and
test the request construction and the response parsing, then say plainly what still needs a real
check in the user's browser.

## The deterministic eval

`make eval` runs the screening prompts against the real model over a fixed set of postings and
exits non-zero when a case or a gate fails. It is never part of `verify` and never part of a round,
because it spends model calls. It needs `PORT` and `DATA_DIR`, and `CV_DIR` the first time, and it
recreates its own settings so nothing the user has configured decides a case.

Everything it judges is derived from the indexed profile at run time: the level comes from the
candidate's own band, the fitting roles from each variant's own core stack, and the foreign
specialisation is the first one this eval knows how to write that no variant carries. A person's
city, stack or resume code never appears in it, so the same eval runs for whoever installs this.

| Named case | Expected | Forbidden |
|---|---|---|
| `core-stack-senior` | apply | skip |
| `below-band-junior` | skip | apply |
| `outside-software` | skip | apply |
| `foreign-specialisation` | skip | apply |
| `instruction-attempt` | apply, with the attempt named in the reason | skip, or a reason that never names it |

A case whose fixture the profile cannot support reports itself unavailable rather than passing.

The distribution gate runs over a larger fixed set built the same way: one fitting role per indexed
variant, plus below-band and foreign counterparts. It fails when fitting roles fall under a 0.60
apply share, when counter roles fall under a 0.80 skip share, or when the set collapses to all
applies or all skips. That last one is the point: `screened` and `picked` only prove the pipeline is
alive, and a prompt edit that quietly makes the screener skip everything passes every liveness
counter in the run. The gate is what refuses it.

Its pure parts are unit tested under `-short` and cost nothing: case construction, the derived
level and lead variant, the identity minting that stops an earlier run's verdict answering for a
new one, and the gate arithmetic in both collapse directions.

## Gates

`make verify` runs every gate and must not change a tracked file:

| Gate | Proves |
|---|---|
| `format-check` | goimports is clean, generated code excluded |
| `generate-check` | the committed sqlc output matches the schema and the query files |
| `mod-check` | `go.mod` is tidy and the module graph verifies |
| `sqlc-vet` | every query file still parses against the authoritative schema |
| `lint` | golangci-lint passes |
| `build` | the module builds |
| `vuln-check` | govulncheck finds nothing |
| `test-unit` | the isolated layer passes under `-short` |
| `test-integration` | the store-backed layer passes |
| `test-race` | the suite passes under the race detector |
| `docker-check` | the image builds and carries `pdftotext` |

Drift is a failure, not something a read-only check repairs. `generate-check` regenerates into a
temporary copy and diffs; it never writes into the tree.

There is no `workflow-check`: this repository has no `.github` directory yet. Add the gate together
with the first workflow, not before it.
