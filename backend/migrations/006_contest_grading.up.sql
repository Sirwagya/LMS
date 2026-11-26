ALTER TABLE assessments ADD COLUMN official_scoring boolean NOT NULL DEFAULT false;

ALTER TABLE assessment_attempts
    ADD COLUMN scoring_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN score_computed_at timestamptz,
    ADD COLUMN choice_saved_at jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(choice_saved_at) = 'object');

CREATE INDEX assessment_attempts_awaiting_score
    ON assessment_attempts(submitted_at, id)
    WHERE scoring_enabled AND submitted_at IS NOT NULL AND score_computed_at IS NULL;

CREATE TABLE assessment_coding_submissions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    request_id uuid NOT NULL,
    question_index int NOT NULL CHECK (question_index BETWEEN 0 AND 49),
    language text NOT NULL CHECK (language IN ('python', 'java')),
    source text NOT NULL CHECK (octet_length(source) BETWEEN 1 AND 32768),
    submitted_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'judging', 'accepted', 'rejected', 'failed')),
    claim_after timestamptz NOT NULL DEFAULT clock_timestamp(),
    generation int NOT NULL DEFAULT 0 CHECK (generation BETWEEN 0 AND 4),
    verdict text,
    UNIQUE (attempt_id, request_id),
    CHECK ((verdict IS NOT NULL) = (status IN ('accepted', 'rejected')))
);
CREATE INDEX assessment_coding_submissions_worker
    ON assessment_coding_submissions(claim_after, id) WHERE status IN ('queued', 'judging');
CREATE INDEX assessment_coding_submissions_attempt
    ON assessment_coding_submissions(attempt_id, question_index, submitted_at, id);

CREATE TABLE assessment_attempt_results (
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    version int NOT NULL CHECK (version > 0),
    status text NOT NULL CHECK (status IN ('ready', 'needs_review')),
    points int,
    possible_points int NOT NULL CHECK (possible_points > 0),
    elapsed_ms bigint,
    question_scores jsonb NOT NULL CHECK (jsonb_typeof(question_scores) = 'array'),
    computed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (attempt_id, version),
    CHECK (
        (status = 'ready' AND points IS NOT NULL AND elapsed_ms IS NOT NULL)
        OR (status = 'needs_review' AND points IS NULL AND elapsed_ms IS NULL)
    ),
    CHECK (points BETWEEN 0 AND possible_points),
    CHECK (elapsed_ms >= 0)
);
