-- name: OpenCycle :execresult
INSERT INTO cycles (started_at, status, rationale, screen_budget) VALUES (?, 'open', ?, ?);

-- name: SaveCycleQuery :exec
INSERT INTO cycle_queries (cycle_id, position, label, query, widened)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(cycle_id, label) DO UPDATE SET query = excluded.query,
  widened = excluded.widened;

-- name: CountCycleQueries :one
SELECT COUNT(*) AS total FROM cycle_queries WHERE cycle_id = ?;

-- name: BumpCycle :exec
UPDATE cycles
   SET received = received + ?, inserted = inserted + ?, duplicates = duplicates + ?,
       widening_steps = widening_steps + ?, model_calls = model_calls + ?,
       input_tokens = input_tokens + ?, output_tokens = output_tokens + ?
 WHERE id = ?;

-- name: BumpCycleQuery :exec
UPDATE cycle_queries
   SET pages = pages + ?, received = received + ?, inserted = inserted + ?,
       duplicates = duplicates + ?
 WHERE cycle_id = ? AND label = ?;

-- name: SkipCycleQuery :execrows
UPDATE cycle_queries SET skipped = TRUE, skip_reason = ? WHERE cycle_id = ? AND label = ?;

-- name: CloseCycle :execrows
UPDATE cycles SET status = ?, finished_at = ?, termination_reason = ?, error = ?
 WHERE id = ? AND status = 'open';

-- name: Cycle :one
SELECT id, started_at, finished_at, status, rationale, screen_budget, received, inserted,
       duplicates, widening_steps, model_calls, input_tokens, output_tokens,
       termination_reason, error
  FROM cycles WHERE id = ?;

-- name: CurrentCycle :one
SELECT id, started_at, finished_at, status, rationale, screen_budget, received, inserted,
       duplicates, widening_steps, model_calls, input_tokens, output_tokens,
       termination_reason, error
  FROM cycles WHERE status = 'open' ORDER BY id DESC LIMIT 1;

-- name: RecentCycles :many
SELECT id, started_at, finished_at, status, rationale, screen_budget, received, inserted,
       duplicates, widening_steps, model_calls, input_tokens, output_tokens,
       termination_reason, error
  FROM cycles ORDER BY id DESC LIMIT ?;

-- name: LastFinishedCycle :one
SELECT id, started_at, finished_at, status, rationale, screen_budget, received, inserted,
       duplicates, widening_steps, model_calls, input_tokens, output_tokens,
       termination_reason, error
  FROM cycles WHERE finished_at IS NOT NULL ORDER BY id DESC LIMIT 1;

-- name: OpenCycleSpend :one
SELECT id, model_calls FROM cycles WHERE status = 'open' ORDER BY id DESC LIMIT 1;

-- name: CycleQueries :many
SELECT label, query, widened, pages, received, inserted, duplicates, skipped, skip_reason
  FROM cycle_queries WHERE cycle_id = ? ORDER BY position ASC;

-- name: InsertNote :exec
INSERT INTO notes (scope, body, priority, created_cycle, updated_cycle, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: UpdateNote :execrows
UPDATE notes SET body = ?, priority = ?, updated_cycle = ?, updated_at = ? WHERE id = ?;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = ?;

-- name: NotesInScope :many
SELECT id, scope, body, evidence, priority, created_cycle, updated_cycle, created_at, updated_at
  FROM notes WHERE scope = ? ORDER BY id DESC LIMIT ?;

-- name: RecentNotes :many
SELECT id, scope, body, evidence, priority, created_cycle, updated_cycle, created_at, updated_at
  FROM notes ORDER BY id DESC LIMIT ?;

-- name: PruneNotes :exec
DELETE FROM notes WHERE notes.scope = ? AND notes.id NOT IN (
  SELECT keeper.id FROM notes AS keeper WHERE keeper.scope = ?
   ORDER BY keeper.id DESC LIMIT ?
);

-- name: BumpModelUsage :exec
INSERT INTO model_usage (day, calls, input_tokens, output_tokens) VALUES (?, ?, ?, ?)
ON CONFLICT(day) DO UPDATE SET calls = model_usage.calls + excluded.calls,
  input_tokens = model_usage.input_tokens + excluded.input_tokens,
  output_tokens = model_usage.output_tokens + excluded.output_tokens;

-- name: ModelUsage :one
SELECT day, calls, input_tokens, output_tokens FROM model_usage WHERE day = ?;
