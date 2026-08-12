-- name: RecentTraces :many
SELECT id, purpose, model, tool_name, state, started_at, finished_at, duration_ms,
       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, error
  FROM model_calls ORDER BY id DESC LIMIT ?;

-- name: RunningTrace :one
SELECT id, purpose, model, tool_name, state, started_at, finished_at, duration_ms,
       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, error
  FROM model_calls WHERE state = ? ORDER BY id DESC LIMIT 1;

-- name: Trace :one
SELECT id, purpose, model, tool_name, state, started_at, finished_at, duration_ms,
       input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, error,
       system_prompt, user_prompt, output
  FROM model_calls WHERE id = ?;
