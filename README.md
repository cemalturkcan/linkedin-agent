# linkedin agent

A job hunt that runs on your own machine. It reads your CV pdfs, works out what you do from those
pdfs alone, searches LinkedIn through your own logged in session, judges what comes back, and puts
the right CV on the application before you press submit.

Nothing about you is written into the code. The same checkout belongs to whoever installs it: your
name, your stack, your cities and the file your CV attaches under are all derived from the folder
you point it at.

[![Watch the 21-second tour](docs/media/promo-poster.png)](https://github.com/cemalturkcan/linkedin-agent/releases/download/promo-v1/promo.mp4)

## What it does

- **Aims the search for you.** Every hour a planner decides where to look, using the places you
  set in settings — which city, how wide a ring around it, how far back — and the profile it
  derived from your CVs. A query that keeps returning nothing gets reworked by the planner, not
  by a rule you maintain.
- **Reads and screens every posting.** Cheap triage on the headers first, a full read of the
  description for the survivors, then a verdict with the stack, the level, the workplace, the
  contract basis and a score. Everything it skips, it skips out loud, with the reason in one
  sentence.
- **Attaches the right CV on its own.** Keep as many variants as you like — `backend/`, `go/`,
  `backend-tr/`. When you open a posting and press Easy Apply, the variant that fits that posting
  is already on the form. It never presses submit; that stays yours.
- **Learns from your outcomes.** Record what an application produced and the reflector turns it
  into standing notes that ride in every later prompt.
- **Stays on your machine.** The store, your CV pdfs and your LinkedIn session never leave it,
  and it runs on the Claude subscription you already have — no key to paste, no second bill.

## Why it exists

Applying for jobs is a queue problem. The feed is endless, most of it is wrong for you, and the
part that is right arrives at three in the morning under a title that names none of your stack. The
work is not writing applications, it is deciding which twelve of four hundred postings deserve one.

So that is the only thing this automates. It never writes a cover letter for you, never answers a
screening question, and never presses submit. It aims the search, throws away what does not fit,
says why in one sentence, and hands you a posting with the right CV already attached.

## How it works

Two pieces, and only two.

**The API** is a Go service on your loopback interface. It owns the store, every prompt, and the
only connection to Claude. It never talks to LinkedIn and holds no LinkedIn credential. It uses your
existing Claude Code credentials, so there is no key to paste and no second subscription.

**The extension** is the interface. A side panel while you work and a desk tab for reading and
configuration. It owns LinkedIn access through your own session cookie, and never talks to
Anthropic. It holds no Claude credential.

A round runs like this:

```
PLAN -> FETCH -> READ -> SCREEN -> RECORD -> LEARN
```

The planner decides where to look: which place, how wide a ring around it, how far back, and why
that ring earns a slot this round. It starts close and widens as the near ground runs dry. The
extension runs that plan against LinkedIn, newest first. Triage judges the headers cheaply, the
survivors get their full description read, and a second pass commits a verdict with the stack, the
level, the workplace, the contract basis and which of your CV variants fits. Everything the round
learned goes back into the next plan, so a query that keeps returning nothing gets reworked by the
planner rather than by a rule you have to maintain.

Your CVs stay on your disk. Their text reaches the model once, during indexing. Everything after
that carries the derived profile instead.

## Install

You need Go 1.26, [Bun](https://bun.sh), Chrome, and Claude Code already signed in.

Build the extension:

```
cd apps/extension
bun install
bun run build
```

Load it in Chrome: open the extensions page, turn on developer mode, choose load unpacked, and pick
`apps/extension/dist`.

Start the API:

```
cd apps/api
go run ./cmd/api
```

It listens on `127.0.0.1:8787` and keeps its database in `~/.local/share/linkedin-agent`. Both are
configurable with `PORT` and `DATA_DIR`.

Then click the extension. On a fresh install it opens the desk on setup and asks for one thing: the
folder holding your CV pdfs.

## The CV folder

One folder, one subfolder per variant, one pdf inside:

```
cv/
  backend/my-cv.pdf
  backend-tr/my-cv.pdf
  frontend/my-cv.pdf
  go/my-cv.pdf
```

The folder name is the variant and a `-tr` style suffix is its language. Indexing reads each pdf
once and derives a profile per variant plus one candidate profile across all of them: years,
seniority, stack, domains, languages and the places you have actually worked. That is what the
planner aims with and what the screener judges against.

If every file in the folder carries the same name, that name becomes the name your CV attaches
under. Employers see it, so it is worth naming the files after yourself rather than leaving them as
`resume.pdf`.

## Using it

The panel has your lists, oldest first, so the posting that has waited longest is at the top. `j`
and `k` move, `enter` opens the posting on LinkedIn, `x` skips it. Open a posting from the inbox and
it moves to the queue: you took it, so it is out of the way.

When a posting is open in the tab you are looking at, the panel follows it and shows which CV is
about to attach and why. Press Easy Apply and the CV goes on by itself. Nothing else on that form is
touched.

When you submit, the agent asks LinkedIn whether the application landed, rather than assuming its
own click worked. Applications made anywhere are picked up the same way, including from your phone,
because the answer comes from LinkedIn rather than from what the extension happened to see.

The desk (`3` or the desk link) holds the rounds and their reasoning, the settings, the derived
profile, every model call with the prompt actually sent, and a log of what the extension did.

## Recording outcomes

The one thing worth doing by hand is telling it what an application produced. Open an applied
posting, press `d`, record the outcome. The reflector reads those outcomes together with the reasons
that produced them and writes durable lessons into the standing notes that ride in every later
prompt. It never rewrites a verdict. Its product is the note.

Without outcomes the agent still plans and screens, it just never learns anything about you it did
not read off your CVs.

## What it will not do

- It does not answer screening questions or write cover letters.
- It does not press submit.
- It does not touch your LinkedIn settings, connections or messages.
- It does not send your CV anywhere except the application form you opened.
- It does not run in the cloud. The store, the pdfs and the session all stay on your machine.

## License

MIT. See [`LICENSE`](LICENSE).

## Documentation

[`docs/`](docs/README.md) holds the contracts: the endpoint surface, the event stream, the round
lifecycle, what a verdict contains, and what LinkedIn actually does when you talk to it. The rules
for changing the code live in [`AGENTS.md`](AGENTS.md) and in the `AGENTS.md` of each component.

## Tests

```
cd apps/api && go test ./...
cd apps/extension && bun test
```

The tests that need real CV pdfs read the folder named by `AGENT_CV_FOLDER` and skip when it is not
set. The ones that need a live LinkedIn session or a live model call are behind their own switches,
because a test suite that quietly spends money or hits someone else's servers is not a test suite.
