-- name: Settings :one
SELECT document FROM settings WHERE id = 1;

-- name: SaveSettings :exec
INSERT INTO settings (id, document, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET document = excluded.document, updated_at = excluded.updated_at;
