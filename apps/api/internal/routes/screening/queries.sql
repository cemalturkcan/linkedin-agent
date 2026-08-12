-- name: Untriaged :many
SELECT id, title, company, location, url, easy_apply, listed_at, description, triage_reason
  FROM jobs WHERE status = 'new' ORDER BY listed_at DESC, created_at DESC LIMIT ?;

-- name: AwaitingDescription :many
SELECT id, title, company, location, url, easy_apply, listed_at, description, triage_reason
  FROM jobs WHERE status = 'reading' AND description_state = 'pending'
 ORDER BY score DESC, listed_at DESC LIMIT ?;

-- name: ReadyForDeepScreen :many
SELECT id, title, company, location, url, easy_apply, listed_at, description, triage_reason
  FROM jobs WHERE status = 'reading' AND description_state IN ('ok', 'missing')
 ORDER BY score DESC, listed_at DESC LIMIT ?;

-- name: SetTriage :execrows
UPDATE jobs
   SET status = 'reading', stage = 'triage', score = ?, triage_reason = ?, screen_key = ?,
       updated_at = ?
 WHERE id = ? AND status = 'new';

-- name: SetVerdict :execrows
UPDATE jobs
   SET status = ?, stage = ?, score = ?, verdict_reason = ?, seniority = ?, workplace = ?,
       contract_type = ?, posting_lang = ?, agency = ?, pay_amount = ?, pay_currency = ?,
       pay_period = ?, resume_code = ?, resume_lang = ?, resume_fit = ?, tailored_needed = ?,
       tailored_focus = ?, screen_key = ?, updated_at = ?
 WHERE id = ?;

-- name: SetStatus :execrows
UPDATE jobs SET status = ?, updated_at = ? WHERE id = ?;

-- name: PutDescription :execrows
UPDATE jobs
   SET description = ?, description_state = ?, described_at = ?, updated_at = ?
 WHERE id = ? AND description_state <> 'ok';

-- name: ExpireDescriptions :execrows
UPDATE jobs SET description_state = 'missing', updated_at = ?
 WHERE status = 'reading' AND description_state = 'pending' AND updated_at < ?;

-- name: EngagedCompaniesSince :many
SELECT company_key, company, status, CAST(MAX(updated_at) AS TEXT) AS engaged_at FROM jobs
 WHERE status IN ('applied', 'opened') AND company_key <> '' AND updated_at >= ?
 GROUP BY company_key;

-- name: SetOutcome :execrows
UPDATE jobs SET status = ?, outcome = ?, outcome_note = ?, outcome_at = ?, updated_at = ?
 WHERE id = ?;

-- name: RecentOutcomes :many
SELECT id, title, company, location, score, verdict_reason, resume_code, resume_fit, seniority,
       workplace, agency, outcome, outcome_note, outcome_at, updated_at
  FROM jobs WHERE outcome IS NOT NULL ORDER BY outcome_at DESC LIMIT ?;

-- name: CountOutcomesSince :one
SELECT COUNT(*) AS total FROM jobs WHERE outcome IS NOT NULL AND outcome_at > ?;

-- name: CountStale :one
SELECT COUNT(*) AS total FROM jobs
 WHERE screen_key IS NOT NULL AND screen_key <> ?
   AND status IN ('inbox', 'manual', 'skipped');

-- name: ResetStale :execrows
UPDATE jobs
   SET status = 'new', stage = 'none', score = NULL, triage_reason = NULL, verdict_reason = NULL,
       seniority = NULL, workplace = NULL, contract_type = NULL, posting_lang = NULL,
       agency = NULL, pay_amount = NULL, pay_currency = NULL, pay_period = NULL,
       resume_code = NULL, resume_lang = NULL, resume_fit = NULL, tailored_needed = NULL,
       tailored_focus = NULL, screen_key = NULL, updated_at = ?
 WHERE screen_key IS NOT NULL AND screen_key <> ?
   AND status IN ('inbox', 'manual', 'skipped');

-- name: LiveResumeCodes :many
SELECT DISTINCT resume_code FROM jobs
 WHERE resume_code IS NOT NULL AND status IN ('inbox', 'manual', 'queue');

-- name: ClearVerdictForResumeCode :execrows
UPDATE jobs
   SET status = 'new', stage = 'none', score = NULL, triage_reason = NULL, verdict_reason = NULL,
       seniority = NULL, workplace = NULL, contract_type = NULL, posting_lang = NULL,
       agency = NULL, pay_amount = NULL, pay_currency = NULL, pay_period = NULL,
       resume_code = NULL, resume_lang = NULL, resume_fit = NULL, tailored_needed = NULL,
       tailored_focus = NULL, screen_key = NULL, updated_at = ?
 WHERE resume_code = ? AND status IN ('inbox', 'manual', 'queue');

-- name: PruneOldJobs :execrows
DELETE FROM jobs
 WHERE status IN ('skipped', 'duplicate') AND outcome IS NULL AND updated_at < ?;

-- name: ScreeningNotes :many
SELECT id, scope, body, evidence, priority, created_at, updated_at
  FROM notes WHERE scope = ? ORDER BY id DESC LIMIT ?;

-- name: InsertLesson :exec
INSERT INTO notes (scope, body, evidence, priority, created_at, updated_at)
VALUES (?, ?, ?, 'normal', ?, ?);

-- name: RetireNote :execrows
DELETE FROM notes WHERE id = ?;

-- name: PruneLessons :exec
DELETE FROM notes WHERE notes.scope = ? AND notes.id NOT IN (
  SELECT keeper.id FROM notes AS keeper WHERE keeper.scope = ?
   ORDER BY keeper.id DESC LIMIT ?
);

-- name: SaveReflection :exec
INSERT INTO reflection (id, ran_at, outcomes, written, retired, model) VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET ran_at = excluded.ran_at, outcomes = excluded.outcomes,
  written = excluded.written, retired = excluded.retired, model = excluded.model;

-- name: LastReflection :one
SELECT ran_at, outcomes, written, retired, model FROM reflection WHERE id = 1;

-- name: BumpModelUsage :exec
INSERT INTO model_usage (day, calls, input_tokens, output_tokens) VALUES (?, ?, ?, ?)
ON CONFLICT(day) DO UPDATE SET calls = model_usage.calls + excluded.calls,
  input_tokens = model_usage.input_tokens + excluded.input_tokens,
  output_tokens = model_usage.output_tokens + excluded.output_tokens;
