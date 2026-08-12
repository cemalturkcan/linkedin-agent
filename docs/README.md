# Contracts

These files own shared behavior. Component documentation owns implementation detail and must not
restate what is decided here.

| Contract | Owns |
|---|---|
| [`http.md`](http.md) | the endpoint surface, the envelope, and what each route is for |
| [`events.md`](events.md) | the event stream: types, payload shape, delivery and recovery |
| [`round.md`](round.md) | the round lifecycle from plan to learn, and how a round ends |
| [`screening.md`](screening.md) | what a verdict contains and which rules the code enforces |
| [`linkedin.md`](linkedin.md) | what LinkedIn actually does, read off a live session |

## The one number

Postings worth applying to per round, with both directions of failure named. A round that fetches
noise burns a budget the person cannot refill. A round that narrows too hard misses the only good
posting of the hour, and nobody ever sees that loss. Every prompt that plans or judges says both.

## Boundaries

The API owns Claude access, the store and every prompt. It never talks to LinkedIn and holds no
LinkedIn credential.

The extension owns LinkedIn access through the user's own session cookie, and is the only user
interface. It never talks to Anthropic and holds no Claude credential.

The person's CV pdfs stay on their disk. Their text reaches the model once, during indexing.
Everything downstream carries the derived profiles instead.
