-- name: ResumeFolder :one
SELECT path FROM resume_folder WHERE id = 1;

-- name: SaveResumeFolder :exec
INSERT INTO resume_folder (id, path, updated_at) VALUES (1, ?, ?)
ON CONFLICT(id) DO UPDATE SET path = excluded.path, updated_at = excluded.updated_at;
