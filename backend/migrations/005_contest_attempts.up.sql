CREATE TABLE assessment_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    assessment_id uuid NOT NULL,
    student_id text NOT NULL,
    contest_revision bigint NOT NULL CHECK (contest_revision > 0),
    device_id uuid NOT NULL,
    lease bigint NOT NULL DEFAULT 1 CHECK (lease > 0),
    started_at timestamptz NOT NULL,
    deadline_at timestamptz NOT NULL CHECK (deadline_at > started_at),
    answers jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(answers) = 'object'),
    answer_revision bigint NOT NULL DEFAULT 0 CHECK (answer_revision >= 0),
    submitted_at timestamptz,
    finish_reason text CHECK (finish_reason IN ('manual', 'expired')),
    UNIQUE (assessment_id, student_id),
    FOREIGN KEY (assessment_id, student_id) REFERENCES assessment_roster(assessment_id, student_id),
    CHECK ((submitted_at IS NULL) = (finish_reason IS NULL)),
    CHECK (submitted_at BETWEEN started_at AND deadline_at)
);

CREATE INDEX assessment_attempts_unfinished_deadline
    ON assessment_attempts(deadline_at, id) WHERE submitted_at IS NULL;

CREATE TABLE assessment_attempt_takeovers (
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    lease bigint NOT NULL CHECK (lease > 1),
    replaced_at timestamptz NOT NULL,
    PRIMARY KEY (attempt_id, lease)
);
