# Event contract

`GET /api/events` is the only push channel. Every component that changes state emits through the
one hub; no package grows its own notification path.

## Delivery

The stream opens with a retry hint and a comment so headers flush at once, and sends a comment
heartbeat often enough that no intermediary reaps an idle connection. Every event carries a
monotonic id. A client that reconnects with the last id it saw is replayed from a bounded ring
buffer, so a dropped connection loses nothing that still matters.

Subscribers are capped. A subscriber whose socket has gone away is dropped, and dropping it frees
its heartbeat rather than leaking one.

## Two kinds of event

A **state event** says what changed and carries only enough to decide. The client refetches the
route that owns the object. This keeps payloads small and keeps one representation of truth.

| Type | Fires when |
|---|---|
| `setup` | the folder or the missing list changed |
| `index` | indexing started, advanced, finished or failed |
| `cycle` | a round was planned, yielded, widened, skipped a query, or finished |
| `jobs` | postings arrived or one moved between lists |
| `screen` | triage or deep screening advanced, or a verdict was written |
| `plugin` | the extension checked in or went quiet |
| `settings` | settings changed |
| `error` | one sentence a person can act on |

A **delta event** is the model's own output arriving while a call is still open. The client appends
it to its own buffer and does not refetch. Deltas are coalesced on a short interval rather than
pushed per token, are bounded per call, and never evict the state events other views depend on.

| Type | Carries |
|---|---|
| `trace` | a model call started, settled or failed |
| `trace-delta` | output from the call that is running |

## What the stream is for

Without it the interface guesses, and a person watching a screen cannot tell working from broken.
Anything that takes longer than a moment reports progress: indexing, a planned round, a harvest, a
screening pass, and the model call itself.

Idle reads as idle. A round that ended because the feed was mined out is a healthy round and says
so, not an error.

## Bursts

A state event names a route to refetch, and a round emits one per verdict it commits. A surface that
refetched on each of them turned a hundred verdicts into a hundred full list reads. Every
event-driven refetch is coalesced instead: a burst inside the window costs one read, and a lone
event still costs its own. The refetch after a stream recovery is not coalesced, because there is
nothing to coalesce it with.
