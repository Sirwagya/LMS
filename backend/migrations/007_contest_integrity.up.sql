CREATE TABLE assessment_reviewers (
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    reviewer_id text NOT NULL,
    assigned_by text NOT NULL,
    assigned_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (assessment_id, reviewer_id)
);
CREATE INDEX assessment_reviewers_reviewer
    ON assessment_reviewers(reviewer_id, assessment_id);

CREATE TABLE assessment_event_batches (
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    batch_id uuid NOT NULL,
    start_seq bigint NOT NULL,
    end_seq bigint NOT NULL,
    server_received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (attempt_id, batch_id)
);

CREATE TABLE assessment_attempt_events (
    id bigserial PRIMARY KEY,
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    seq bigint NOT NULL CHECK (seq > 0),
    batch_id uuid,
    source_kind text NOT NULL CHECK (source_kind IN ('server', 'client')),
    event_type text NOT NULL CHECK (event_type IN (
        'attempt.started',
        'attempt.takeover',
        'attempt.saved',
        'attempt.submitted',
        'attempt.expired',
        'attempt.code_submitted',
        'nav.question',
        'mcq.selected',
        'code.snapshot',
        'code.delta',
        'code.run',
        'editor.paste',
        'window.focus',
        'window.blur',
        'fullscreen.enter',
        'fullscreen.exit',
        'heartbeat.gap',
        'client.reconnect'
    )),
    question_index int CHECK (question_index IS NULL OR question_index BETWEEN 0 AND 49),
    client_at timestamptz,
    server_received_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    payload jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 8192),
    UNIQUE (attempt_id, seq)
);
CREATE INDEX assessment_attempt_events_attempt_seq
    ON assessment_attempt_events(attempt_id, seq);

CREATE TABLE assessment_reviews (
    attempt_id uuid PRIMARY KEY REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'clear' CHECK (status IN ('clear', 'flagged', 'under_review', 'cleared', 'confirmed_violation')),
    signals jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(signals) = 'array'),
    similarity_max real NOT NULL DEFAULT 0 CHECK (similarity_max BETWEEN 0 AND 1),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 2000),
    reviewer_id text,
    reviewed_at timestamptz,
    retain_until timestamptz NOT NULL DEFAULT (clock_timestamp() + interval '30 days'),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX assessment_reviews_retain_until
    ON assessment_reviews(retain_until);

CREATE TABLE assessment_appeals (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    student_id text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('concern', 'appeal')),
    message text NOT NULL CHECK (char_length(trim(message)) BETWEEN 1 AND 2000),
    status text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_review', 'resolved', 'dismissed')),
    reviewer_id text,
    resolution text NOT NULL DEFAULT '' CHECK (char_length(resolution) <= 2000),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    resolved_at timestamptz
);
CREATE INDEX assessment_appeals_attempt_created
    ON assessment_appeals(attempt_id, created_at DESC, id);
CREATE INDEX assessment_appeals_open_status
    ON assessment_appeals(attempt_id) WHERE status IN ('open', 'in_review');

CREATE TABLE assessment_similarity_pairs (
    id bigserial PRIMARY KEY,
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    question_index int NOT NULL CHECK (question_index BETWEEN 0 AND 49),
    attempt_a uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    attempt_b uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    similarity real NOT NULL CHECK (similarity BETWEEN 0 AND 1),
    matched_tokens int NOT NULL CHECK (matched_tokens >= 0),
    snippet_a text NOT NULL DEFAULT '',
    snippet_b text NOT NULL DEFAULT '',
    computed_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK (attempt_a < attempt_b),
    UNIQUE (assessment_id, question_index, attempt_a, attempt_b)
);
CREATE INDEX assessment_similarity_pairs_assessment
    ON assessment_similarity_pairs(assessment_id, similarity DESC, id);
CREATE INDEX assessment_similarity_pairs_attempts
    ON assessment_similarity_pairs(attempt_a, attempt_b);

CREATE TABLE assessment_evidence_access_log (
    id bigserial PRIMARY KEY,
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    attempt_id uuid REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    actor_id text NOT NULL,
    action text NOT NULL,
    detail text NOT NULL DEFAULT '',
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX assessment_evidence_access_log_assessment
    ON assessment_evidence_access_log(assessment_id, occurred_at DESC, id DESC);
