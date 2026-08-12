# HTTP contract

The API listens on the loopback interface only. It serves JSON and one event stream, and it serves
no user interface: the extension is the interface.

The extension's origin is the only cross-origin caller. Preflight is answered for it and for
nothing else.

## Envelope

A successful response is the resource itself. A failure is `{"error": "<one sentence>"}` with a
status that matches the cause: 400 for a malformed request, 404 for something that does not exist,
409 for a state that forbids the action, 503 for a dependency that is not ready.

Error text follows the copy rules in the root `AGENTS.md`: what failed, and the one action that
fixes it when there is one. Never a raw driver or provider message.

## Setup and identity

| Route | Purpose |
|---|---|
| `GET /api/health` | liveness plus the queue counts. Never carries plan or tier information |
| `GET /api/setup` | what is configured, what is missing by name, and folder candidates |
| `POST /api/setup` | set the CV folder. Starts indexing in the background and returns at once |
| `GET /api/credentials` | which source answered and whether credentials are loaded. Never the value |
| `PUT /api/credentials` | store pasted credentials |
| `DELETE /api/credentials` | forget stored credentials, effective without a restart |
| `POST /api/plugin/hello` | the extension announces its id, version and capabilities |
| `GET /api/plugin` | connected state, last seen, and the capability manifest |

Credentials resolve from an environment variable, then the store, then the local Claude Code file.
The token never appears in a response, a log line, an error or an event.

## Resumes and derivation

| Route | Purpose |
|---|---|
| `GET /api/resumes` | the resume list derived from the folder layout |
| `GET /api/resumes/:code/:lang/file` | the pdf bytes, read from disk at request time |
| `GET /api/profile` | the candidate profile, the per-resume profiles, and the index state |
| `POST /api/index` | index the folder. Unchanged files cost no model call |

Index state is `never`, `stale` or `current`, and those are three different sentences to the user.
Never indexed is not an error, and indexing in progress is not an error at all.

## Settings

| Route | Purpose |
|---|---|
| `GET /api/settings` | the settings object and the enumerations the interface renders from, including the two upload naming modes |
| `PUT /api/settings` | deep merge, validated server side, rejecting nothing silently |

The upload file name has two honest modes. `one-name` attaches every variant under the one name the
person chose, which is what an employer sees and which means the extension cannot tell stored files
apart, so it always uploads. `per-variant` gives each variant its own name, which lets the extension
select a copy LinkedIn already holds instead of uploading another. It ships on `one-name`, because
attaching the wrong resume is worse than a duplicate.

The name itself ships empty, and empty means the person has not chosen one. `GET` then answers with
the name their own cv files carry when every file in the folder carries the same one, and with
`resume.pdf` when they disagree. It costs no model call: it is the file name on disk, not a reading
of the cv. The same rule already fills the resume languages from the folder. Nothing that narrows
the search is filled this way: places stay empty and the seniority range stays open, because those
bound what the planner may look for and that is the person's call.

Every field ships with a neutral default. Locations start empty, and empty means the planner
decides rather than blocked. Workplace defaults wide: onsite, hybrid and remote all acceptable,
scope global, relocation open.

## The round

| Route | Purpose |
|---|---|
| `POST /api/cycle/start` | plan a round, or refuse with a reason and spend nothing |
| `POST /api/cycle/widen` | one widening step, chosen by the planner |
| `POST /api/cycle/skip` | a planned query could not run, with the reason |
| `POST /api/cycle/finish` | close the round with its termination reason |
| `GET /api/plan` | the current round for reading. Never plans, never spends |

A refusal is `{"ok": false, "reason", "message"}` with 409. Reasons include the executor never
having checked in, the daily model call cap, the open round's own model call ceiling, and the pause
switch. Planning spends no model call when it is going to refuse.

Those reasons are gate state: they clear when the thing that caused them clears, and the interface
drops them from the screen as soon as `GET /api/plan` reports no block. `planner-failed` is not
gate state. It records an attempt that already happened and spent a model call, so it opens a round,
closes it as failed with that termination reason, and stays on screen until the person dismisses it.

## Postings

| Route | Purpose |
|---|---|
| `POST /api/jobs` | harvest results, tagged with the query that found them |
| `GET /api/jobs` | the lists the interface shows |
| `POST /api/jobs/opened` | postings the person opened themselves, held against the agent |
| `GET /api/jobs/pending-descriptions` | the ids the screener wants read in full |
| `POST /api/jobs/descriptions` | the body text for those ids |
| `POST /api/jobs/screen` | run screening over what is unjudged |
| `POST /api/jobs/:id/<transition>` | move one posting between lists |
| `POST /api/jobs/:id/outcome` | record what the application actually produced |
| `GET /api/jobs/stale` | what predates a settings or CV change |
| `POST /api/jobs/rescreen` | judge the stale ones again |

A posting already handled is dropped before batching, no matter who sends it or how often. That
guarantee lives on the server; the caches on the other side are optimisations.

Handled is every status but `new`. `POST /api/jobs/opened` records what the person opened for
themselves: it stores the posting if the store has never seen it, marks it `opened` at stage `hand`
with whichever cv was armed, and leaves a posting the agent already owns exactly as it was. An
opened posting is never screened, its company rides the reapply cooldown, and it takes an outcome
like any other row.

## Places, notes, reflection and traces

| Route | Purpose |
|---|---|
| `GET /api/places` | places already resolved, for the interface to offer |
| `POST /api/places` | a resolution the extension obtained from LinkedIn |
| `GET /api/notes` | the standing notes that ride in every prompt |
| `DELETE /api/notes/:id` | delete a note the agent got wrong |
| `POST /api/reflect` | turn recorded outcomes into lessons |
| `GET /api/traces` | recent model calls: purpose, timing, tokens, state |
| `GET /api/traces/:id` | one call in full, including the prompt actually sent |
| `GET /api/prompt/preview` | the assembled prompts exactly as the model will see them |

The preview is the honest text, never a paraphrase. A person who writes operator notes has to be
able to read what their words did.
