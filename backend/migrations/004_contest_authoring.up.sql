CREATE TABLE assessments (
    id uuid PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title text NOT NULL,
    instructions text NOT NULL,
    opens_at timestamptz NOT NULL,
    closes_at timestamptz NOT NULL,
    duration_minutes int NOT NULL CHECK (duration_minutes BETWEEN 1 AND 720),
    questions jsonb NOT NULL CHECK (jsonb_typeof(questions) = 'array'),
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'cancelled')),
    published_at timestamptz,
    cancelled_at timestamptz,
    cancelled_by text REFERENCES users(id) ON DELETE RESTRICT,
    cancellation_reason text,
    CHECK (closes_at >= opens_at + duration_minutes * interval '1 minute')
);
CREATE INDEX assessments_owner_opens ON assessments(owner_id, opens_at DESC, id DESC);
CREATE INDEX assessments_opens ON assessments(opens_at DESC, id DESC);
CREATE INDEX assessments_cancelled_by ON assessments(cancelled_by) WHERE cancelled_by IS NOT NULL;

CREATE TABLE assessment_roster (
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    student_id text NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    PRIMARY KEY (assessment_id, student_id)
);
CREATE INDEX assessment_roster_student ON assessment_roster(student_id, assessment_id);
