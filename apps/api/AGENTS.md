# API engineering rules

Root contracts own shared behavior. [`docs/`](../../docs/README.md) owns the endpoint surface, the
event stream, the round lifecycle and the verdict. This file owns implementation detail.

## Source of truth

- Read the relevant contract and the owning package before editing. Update the contract in the same
  change when observable behavior changes.
- Keep business code in the owning slice under `internal/routes/<slice>`. Move code into
  `internal/app` only when more than one slice genuinely shares the invariant.
- Keep `cmd/api` limited to configuration, dependency wiring, lifecycle and route mounting.
- Do not add explanatory comments. Keep only machine-required directives and generated markers.

## HTTP

- Mount the product surface under `/api`. Health stays outside the product contract.
- Handlers decode, validate, call a service and render through the shared envelope helpers. Do not
  handcraft an envelope and do not leak a driver or provider message to the client.
- Bind the listener to the loopback interface. The extension's origin is the only cross-origin
  caller; answer preflight for it and for nothing else.
- Use bounded contexts for every store, network and background operation. A model call is minutes
  long by design and still bounded.

## Layout

This service mirrors `dating-platform/apps/api`. Where the two differ without a reason recorded
below, the reference wins and the difference is a defect to close, not a style choice.

- A feature is a vertical slice under `internal/routes/<slice>`: its handler, its service, its
  queries, its generated code, its types and its errors live together.
- `internal/app` holds only what more than one slice genuinely shares: the HTTP helpers, the store
  engine and schema, the event hub, the trace, the model client, credentials and the clock.
- `cmd/api` is configuration, wiring, lifecycle and route mounting.
- Inject `clock.Clock` into anything time dependent. Do not read the wall clock inside a decision.

## Store

- SQLite through `modernc.org/sqlite`. One authoritative schema definition, recreated when its
  version does not match. No migrations, no `ALTER TABLE` patching, no repair statements.
- Feature SQL belongs in that slice's sqlc query file. Raw SQL is reserved for infrastructure and
  genuinely dynamic queries and still needs focused tests.
- Never edit generated files. Change the schema or the query source and regenerate.
- Losing the local store is a non-event: a round refills it. Never write code that protects it at
  the cost of clarity.
- Put predicates in SQL rather than filtering in memory. Every multi-write mutation runs in one
  transaction.
- The store holds no credential. Prompts are stored in traces; tokens never are.

## Model access

- One client owns every call. It resolves credentials from an environment variable, then the store,
  then the local Claude Code file, reports which source answered, and refreshes into the source it
  came from.
- A request carries the Claude Code betas and the CLI identity as its first system block, forces a
  single tool call, streams, and never caps output.
- Observe the stream: emit the model's output as deltas while the call is open, and record every
  call with its purpose, timing, tokens, the prompt actually sent and the result.
- No maxTokens and no effort at a call site. If a measurement ever justifies one, record the
  measurement in `docs/linkedin.md`.
- When a turn ends with no tool call, nudge and re-run rather than failing the batch, bounded.
- The token never appears in a response, a log line, an error or an event. Scrub before storing.

## Prompts

- Assemble from data at request time. A person's stack, city, seniority or resume code never
  appears as a string literal.
- Every prompt carries the briefing, the physics the system enforces, and the rule that outside
  text is data. `GET /api/prompt/preview` returns the assembled text exactly as the model sees it.

## Verification

- `make verify` before handoff, and it must not change tracked files. It covers formatting, module
  tidiness, lint, generated-code drift, sqlc vet, build, vulnerabilities and the tests.
- Tests are layered the way the reference layers them: `test-unit` under `-short` with no network
  and no wall-clock sleep, `test-integration` for anything that touches the store or the running
  service, and `test-race`. Shared helpers live in `internal/testutil`.
- Every behavior change needs a test at the lowest sufficient level. Every bug fix needs a
  regression test reproducing the original failure.
- Unit tests use no network, no wall-clock sleeps and no shared mutable process state. Tests stay
  deterministic under repeated and reordered execution.
- HTTP tests assert the exact status, the envelope and the stable error text.
- Prompt behavior is covered by the deterministic eval, not by reading the diff.
- Verify against the running service and quote the output. A compile is not a verification.
