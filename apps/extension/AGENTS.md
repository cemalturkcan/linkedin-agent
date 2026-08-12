# Extension engineering rules

Root contracts own shared behavior. [`docs/`](../../docs/README.md) owns the endpoint surface, the
event stream and the round lifecycle. This file owns implementation detail.

The extension is the only user interface and the only component with LinkedIn access. It runs in
the user's own Chrome with their own session cookie, and holds no Claude credential.

## Surfaces

Two entry points, because one of them is 400 pixels wide and the other is not.

- **Side panel** is the doing surface: the agent's lists, the presets, the armed resume, round
  control, per-posting actions, live status, and the manual LinkedIn feed. It is what is open while
  the person is on LinkedIn.
- **Desk page** is a full extension tab. It opens on the work: every posting the agent found and
  judged, with the verdict, the score, the reason, the cv it assigned and how well that cv fits, the
  tailored focus when there is one, the round and query that found it, and the control that records
  what an application produced. Long text belongs here, not in the panel.

The panel's fifth tab is that manual feed: LinkedIn's own newest-first results for a place chosen
by name through the typeahead, a time window, easy apply and remote, paged by hand. No keyword is
ever sent and no title decides anything. Postings the system has never recorded are marked unseen
and counted on the toolbar badge; everything handled is absent, agent and person alike. Opening one
records it as handled with whichever cv is armed, so the agent never spends a model call on it and
it still counts for the cooldown, the duplicate collapse and the outcome.

Opening a posting, from either the manual feed or the agent's lists, opens a background tab and
never steals focus: he decides when to go there, and the open is what marks it handled.

The panel follows the active tab rather than being navigated. When the active tab is a LinkedIn
posting the panel shows the resume step for that posting: which cv attaches, whether that is the
agent's own pick, the cv he armed or nothing, and the presets to change it. When the active tab is
anything else the panel goes back to the list by itself, on activation and on navigation alike. A
tab that is still loading and does not yet name a posting holds the screen it had, so nothing
flickers. One rule owns the conflict: automatic switching owns the screen while he has not touched
the panel, and a screen he chose himself holds until the active tab changes again. There is one way
to recognise a posting, `postingIdFrom`, and the content script reports its href rather than
carrying a second copy of it.

The panel links into the desk. Neither surface keeps a second copy of what the API owns: the handled
set is the API's, cached here only so both surfaces still hide the right rows while it is down.

The desk carries three views, reachable by number: `1` work, `2` rounds, `3` settings. Configuration
is not a peer of the work: settings is the one place it lives, and setup, the derived profile and the
trace are sections under it. The letters the panel already uses still land where they always did:
`o` work, `.` rounds, `,` settings, `c` setup, `p` profile, `m` trace, `l` log. The panel opens one
by hash, so `desk.html#settings` lands on the settings view and `desk.html#setup` on its setup
section. `j` and `k` walk whatever list the view holds, `enter` opens the posting on LinkedIn, `d`
opens its detail here, `escape` closes it, `r` refetches, `i` re-reads the cvs and `shift+p` is the
pause switch. Enter goes to the posting because that is what a person on a list of jobs wants, and
the detail is a key of its own rather than the thing standing in the way.

A surface fills itself once when it mounts and once more only when the stream comes back after
dropping. The first connect is not a recovery: the data it would refetch was read milliseconds
earlier, and refetching there doubled every screen's load. Recovery is a stream that was live, went
down and came back, which is the only case where what is on screen can be behind.

Opening a posting is an act, and both surfaces record it the same way: the tab opens and a posting
that was proposed, in the inbox or waiting to be sent by hand, moves to the queue. One rule, one
place, so the panel and the desk cannot drift. That is why a list shrinks when you press enter on
it: the posting is not lost, it is queued, and the queue is where the ones you took stand until
LinkedIn says they were applied to.

The work view narrows twice: by list, and by the cv the agent picked for the posting. The cv chips
are drawn from the postings on screen, they carry the count, and they read as the variant's own
name, so narrowing to the java ones is one click rather than a search.

Nothing is set up on a fresh install, and an empty panel is a bad first sentence. When the api says
no cv folder is configured the panel opens the desk on setup and says so in one line rather than
drawing four empty lists.

A refusal to attach is not the end of the matter. The only outcome that settles the cv step is a cv
that already went on, because everything else names something the person can change while the modal
is still open: arm a variant, unpause, switch attaching back on, fix the folder. So the watcher keeps
offering, and it retries at once when a cv is armed rather than waiting out its own gap. Refusing
once and going quiet is how a person ends up staring at a step that says it will attach and never
does.

The log section records what the extension did: the rounds it ran, every cv it attached or refused
to attach and the reason, and the times the agent did not answer. It records by default, it is kept
in the browser and sent nowhere, it holds the last few hundred lines, and it can be switched off and
cleared from its own screen. The panel says the same thing in one line on the posting it is
following, so a cv that did not attach names its reason where the person is already looking.

The default view is decided once, by the setup chain, and a click or a hash always wins over it: the
desk lands on the work when the chain holds and on setup when a link is short, because a short chain
has nothing else to show. Nobody is ever bounced out of a screen they navigated to.

Both surfaces order postings oldest first, first in and first out, so the longest wait is worked
first and nothing rots at the bottom of a growing list. A posting's age is its listing time, falling
back to when it arrived if the feed gave none, which is the server's own ordering reversed.

Every view marks itself cached when its resource is stale, and draws nothing at all when the agent
has never answered: an empty list and an unanswered one are different sentences, and neither is ever
drawn as the other.

## Stack

React 19, Vite 8, Tailwind 4, shadcn (new-york), TypeScript. Vite builds both entry points into the
unpacked extension. No remote code, no CDN, no plain hand-written HTML pages.

Bun 1.3 is the package manager and the task runner: `bun install`, `bun run dev`, `bun run build`,
`bun test`. Only `bun.lock` is committed; there is no npm lockfile. The built output is ordinary
JavaScript that Chrome loads, so the runtime is unaffected by the choice.

The service worker and the content script stay small and dependency-free. Only the surfaces go
through the bundle.

## Design

Tokens come from the root `AGENTS.md`, and they come from the user's own CV documents: a warm
`#f7e7dc` ground, near-black ink, muted secondary text, hairline rules, a slightly raised accent
for surfaces and selection. Uppercase display headings, Roboto body, Inter for dense grids.

Dense, quiet, keyboard first. No left-border rails, no gradients, no emoji, no decorative motion.
The shortcuts from the previous interface carry over unchanged; a person who learned them keeps
them.

One motion is intended: a posting the person opens slides out of the list, because it is handled.
Short, single, no bounce, and it must not leave a jumping gap or fight the keyboard.

## Behavior

- Work with the API down. Say so plainly, keep the manual path alive, and never present a cached
  value as live.
- Consume the event stream rather than polling. Reconnect with the last id seen. A delta appends to
  a local buffer; a state event refetches the route it names.
- A round must always close, even when the worker is reclaimed mid-round. Hold the worker for the
  duration of a round, drop the hold when it ends, resolve an orphaned round on the next wake, and
  report why a round died.
- Be a good citizen of LinkedIn: space requests out, honour a rate limit with backoff, stop on a
  lost session and say so, and never let paging look like a scraper.
- At the form the extension attaches a resume and stops. It never clicks next, review or submit,
  never answers a screening question and never fills a field. There is no auto-submit mode and no
  setting that leads to one.
- Prefer selecting a resume LinkedIn already holds over uploading another copy. Match exactly or
  upload; attaching the wrong resume is worse than a duplicate.

## Verification

- `bun test` for the units that can be tested here, `bun run build` for the surfaces, and
  `manifest.json` must parse.
- Nothing in this repository can call LinkedIn: only the user's browser holds the session. Test the
  request construction and the response parsing against captured shapes, then say plainly what
  still needs a real check in the user's browser. Never claim something works against LinkedIn.
- Do not write comments. Write self-documenting code.
