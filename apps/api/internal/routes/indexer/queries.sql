-- name: ResumeProfiles :many
SELECT code, hash, profile, indexed_at FROM resume_profiles ORDER BY code;

-- name: SaveResumeProfile :exec
INSERT INTO resume_profiles (code, hash, profile, indexed_at) VALUES (?, ?, ?, ?)
ON CONFLICT(code, hash) DO UPDATE SET profile = excluded.profile,
  indexed_at = excluded.indexed_at;

-- name: PruneResumeProfiles :exec
DELETE FROM resume_profiles
 WHERE code || ':' || hash NOT IN (SELECT value FROM json_each(?));

-- name: DeleteResumeProfiles :exec
DELETE FROM resume_profiles;

-- name: CandidateProfile :one
SELECT profile, source_key, model, indexed_at FROM candidate_profile WHERE id = 1;

-- name: SaveCandidateProfile :exec
INSERT INTO candidate_profile (id, profile, source_key, model, indexed_at)
VALUES (1, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET profile = excluded.profile, source_key = excluded.source_key,
  model = excluded.model, indexed_at = excluded.indexed_at;
