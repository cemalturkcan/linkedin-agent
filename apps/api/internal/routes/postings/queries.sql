-- name: HandledIDs :many
SELECT id FROM jobs
 WHERE id IN (SELECT value FROM json_each(?))
   AND status IN (SELECT value FROM json_each(?));

-- name: CanonicalFor :one
SELECT id FROM jobs WHERE dupe_key = ? AND dupe_key <> '' AND status <> ?
 ORDER BY created_at ASC LIMIT 1;

-- name: InsertJob :execrows
INSERT OR IGNORE INTO jobs
  (id, title, company, location, url, company_key, dupe_key, dupe_of, description,
   description_state, described_at, listed_at, easy_apply, status, stage, resume_code,
   resume_lang, cycle_id, query_label, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: MarkOpened :execrows
UPDATE jobs
   SET status = 'opened', stage = 'hand', resume_code = ?, resume_lang = ?, updated_at = ?
 WHERE id = ? AND status = 'new';

-- name: Jobs :many
SELECT id, title, company, location, url, dupe_of, description, description_state,
       listed_at, easy_apply, status, stage, score, triage_reason, verdict_reason,
       seniority, workplace, contract_type, posting_lang, agency, pay_amount, pay_currency,
       pay_period, resume_code, resume_lang, resume_fit, tailored_needed, tailored_focus,
       outcome, outcome_note, cycle_id, query_label, created_at, updated_at,
       CAST(screen_key IS NOT NULL AND screen_key <> ?
        AND status IN ('inbox', 'manual', 'skipped') AS BOOLEAN) AS stale
  FROM jobs ORDER BY listed_at DESC, created_at DESC;

-- name: JobsByStatus :many
SELECT id, title, company, location, url, dupe_of, description, description_state,
       listed_at, easy_apply, status, stage, score, triage_reason, verdict_reason,
       seniority, workplace, contract_type, posting_lang, agency, pay_amount, pay_currency,
       pay_period, resume_code, resume_lang, resume_fit, tailored_needed, tailored_focus,
       outcome, outcome_note, cycle_id, query_label, created_at, updated_at,
       CAST(screen_key IS NOT NULL AND screen_key <> ?
        AND status IN ('inbox', 'manual', 'skipped') AS BOOLEAN) AS stale
  FROM jobs WHERE status = ? ORDER BY listed_at DESC, created_at DESC;

-- name: Job :one
SELECT id, title, company, location, url, dupe_of, description, description_state,
       listed_at, easy_apply, status, stage, score, triage_reason, verdict_reason,
       seniority, workplace, contract_type, posting_lang, agency, pay_amount, pay_currency,
       pay_period, resume_code, resume_lang, resume_fit, tailored_needed, tailored_focus,
       outcome, outcome_note, cycle_id, query_label, created_at, updated_at,
       CAST(screen_key IS NOT NULL AND screen_key <> ?
        AND status IN ('inbox', 'manual', 'skipped') AS BOOLEAN) AS stale
  FROM jobs WHERE id = ?;

-- name: JobCounts :many
SELECT status, COUNT(*) AS total FROM jobs GROUP BY status;

-- name: RecentJudgedIDs :many
SELECT id FROM jobs WHERE status <> ? AND created_at >= ?
 ORDER BY created_at DESC LIMIT ?;

-- name: SavePlace :exec
INSERT INTO places
  (place_key, name, asked, ring, location_id, source, hits, first_seen_at, last_seen_at)
VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT(place_key) DO UPDATE SET name = excluded.name,
  asked = CASE WHEN excluded.asked <> '' THEN excluded.asked ELSE places.asked END,
  ring = CASE WHEN excluded.ring <> '' THEN excluded.ring ELSE places.ring END,
  location_id = excluded.location_id, source = excluded.source,
  hits = places.hits + 1, last_seen_at = excluded.last_seen_at;

-- name: Place :one
SELECT name, ring FROM places WHERE place_key = ?;

-- name: Places :many
SELECT place_key, name, asked, ring FROM places ORDER BY hits DESC, last_seen_at DESC;

-- name: CountPlaces :one
SELECT COUNT(*) AS total FROM places;

-- name: PrunePlaces :exec
DELETE FROM places WHERE place_key NOT IN (
  SELECT place_key FROM places ORDER BY last_seen_at DESC LIMIT ?
);
