# Round contract

A round is one pass of the loop. The API plans it, the extension executes it, and the API records
what happened so the next plan is better than the last.

```
PLAN -> FETCH -> READ -> SCREEN -> RECORD -> LEARN
```

## Plan

One forced tool call. The prompt is assembled at request time from the derived candidate and
resume profiles, the executor's own capability manifest, the user's settings, the per-query yield
of recent rounds, the standing notes, and the operator notes.

A query names a place the way a person would and the ring it works:

| Ring | Reach |
|---|---|
| `city` | one city |
| `country` | one country |
| `region` | a multi-country region |
| `worldwide` | no place at all |

The planner spends the round's budget across rings, starts close and widens as the near rings run
dry, and each query's reason names its ring and why it earns a slot this round. Where the person
has been working comes from their CVs, not from a form they filled in. Configured places bound the
planner; an empty list means the planner decides.

A query carries a keyword, and the planner writes it from the candidate and resume profiles it
already holds. The executor sends that term to LinkedIn as LinkedIn's own matching. Nothing is
filtered locally: everything LinkedIn returns for the term still reaches triage, which reads every
header against the derived profile.

The prompt states the tradeoff rather than hiding it. LinkedIn's keyword matching is loose, so a
term narrows the fetch without guaranteeing relevance, and it can miss an advert that never spells
its stack out. A term chosen badly costs postings nobody ever sees; it cannot cost a posting that
arrived. An empty term is not an error, it is a query that reads the raw feed for that place and
window, which is the right call when the place is tight and the window short.

There is still no title filter. A posting titled `Senior Software Engineer` can be the strongest
match of the week and name none of the stack it runs on, so nothing between the feed and the
screener throws a posting away for the words in its title.

The same terms, and the places the CVs name, are offered in the manual feed as one-click chips, so
the person aims by hand with what their own resumes carry.

## Fetch

The extension runs the plan against LinkedIn with the user's own session cookie, newest first. It
works the near rings before the far ones, city then country then region then worldwide, whatever
order the plan lists them in, so a worldwide query can never spend the round's target before the
tight ground has been worked. Ordering inside one ring is the planner's. It narrows nothing:
what a query returns is what the screener sees. A place name is resolved to
LinkedIn's own identifier by the extension; worldwide carries no location clause at all. A name
LinkedIn does not know skips its query, reports why, and the next plan sees it.

Handled postings never reach the model. The extension filters what it has seen, the plan carries a
bounded digest of recently handled ids for a fresh install, and the server drops anything already
handled. The first two are optimisations; the third is the guarantee.

Handled is one notion and the API owns it: every posting whose status is not `new`, whether the
screener judged it, the person queued it, or the person opened it themselves from the manual feed.
The extension caches the set so both surfaces still hide them while the API is down.

## Read and screen

Screening is two stages, and the round runs both itself. When the executor stops fetching it calls
screening over what it just brought back: triage judges the header cheaply and decides what
deserves reading, the extension then fetches the body text for the survivors, and a second
screening pass judges the real stack, level, workplace, contract basis, stated pay and language
before it commits a verdict. Nobody presses anything, and both passes happen while the round is
still open so every call is charged to it.

Every model call screening spends is charged to the open round, and the round carries a model call
ceiling the person sets. Screening reads the round's spend before each batch and stops at that
boundary once the ceiling is reached: it never truncates a batch and never leaves a posting half
judged. It records why it stopped, and the postings it did not reach stay unjudged so the next
round takes them. A ceiling of 0 is no ceiling.

## Record and learn

Per-query yield, screening totals and model usage are written to the round. They feed the next
plan, so a query that keeps returning nothing gets reworked by the planner rather than by a rule.

A posting the person opened by hand is recorded with a status that says so. It is never screened,
its company rides the reapply cooldown, it collapses onto the role already on file the way any
harvest does, and an outcome is recorded on it exactly as on one the agent queued.

An application is the person's act, not the agent's, and it can happen anywhere: in the apply modal
the extension is watching, in another tab, or on a phone. So applied is not inferred from what the
extension saw happen. LinkedIn is asked. The apply watcher asks the moment the cv step closes, and
every round starts by sweeping the postings the person still has open, queued or marked manual and
asking LinkedIn which of them are applied. A posting that answers yes moves to applied here. The
sweep is bounded, it re-asks the same posting at most twice an hour, and it says in the log how many
it left for the next one. A check that found nothing blocks the next check for seconds, never hours:
the whole point is to be asking again right after the person presses submit.

When the person records what an application produced, the reflector reads those outcomes together
with the reasons that produced them and writes durable lessons into the standing notes, routing
each to planning or to screening. It never changes a verdict; its product is the note.

## Ending

A round must find new work, and it must stop.

When new postings fall short of the target, the executor pages deeper on queries still producing,
moves to the next query, then asks the planner for one widening step, bounded.

A round ends on the first of: the target reached, every query exhausted, a full page with nothing
new, the widening steps spent, the deadline, a rate limit or a lost session, the daily model call
cap, its own model call ceiling, or the pause switch. The reason is recorded and shown.

A query the compiler rejects is not silently dropped. It is written into the round as a query that
never ran, carrying what the planner wrote and a reason that begins by saying it was rejected before
it ran, so the next plan reads its own mistake back in the same block it reads yields from. The
round still runs on the queries that survived: one malformed query costs its own slot, never the
round.

A plan with no usable query is not a healthy round. The round is opened, recorded as failed with
the termination reason `planner-failed`, and closed at once, so the spent model call and the empty
answer are both visible where every other round's reason is shown. A failed plan is not evidence
about where to look, so it is kept out of the history the planner reads while staying in the
history the person reads.

A round can also die: the browser closes, the worker is reclaimed, a call throws. It must still
end. The executor always closes a round it opened, resolves an orphan on its next wake, and the
API closes a round whose deadline has passed. A round that vanishes silently is the failure this
rule exists to prevent.
