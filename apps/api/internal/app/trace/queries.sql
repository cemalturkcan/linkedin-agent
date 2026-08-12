-- name: OpenTrace :execresult
INSERT INTO model_calls (purpose, model, tool_name, state, started_at, system_prompt, user_prompt)
VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: PruneTraces :exec
DELETE FROM model_calls
 WHERE id NOT IN (SELECT id FROM model_calls ORDER BY id DESC LIMIT ?);

-- name: CloseTrace :exec
UPDATE model_calls
   SET state = ?, finished_at = ?, duration_ms = ?, model = ?, input_tokens = ?,
       output_tokens = ?, cache_read_tokens = ?, cache_write_tokens = ?, output = ?, error = ?
 WHERE id = ?;

-- name: ResolveInterrupted :exec
UPDATE model_calls SET state = ? WHERE state = ?;
