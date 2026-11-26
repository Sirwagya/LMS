ALTER TABLE assessments
    ADD COLUMN release_explanations boolean NOT NULL DEFAULT false,
    ADD COLUMN latest_result_version int NOT NULL DEFAULT 0 CHECK (latest_result_version >= 0),
    ADD COLUMN results_published_at timestamptz;

ALTER TABLE assessment_roster
    ADD COLUMN display_alias text NOT NULL DEFAULT '' CHECK (char_length(display_alias) <= 64);

CREATE TABLE assessment_result_releases (
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    version int NOT NULL CHECK (version > 0),
    published_by text NOT NULL,
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 2000),
    release_explanations boolean NOT NULL DEFAULT false,
    withheld_attempt_ids jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(withheld_attempt_ids) = 'array'),
    published_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (assessment_id, version)
);

CREATE TABLE assessment_leaderboard_entries (
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    version int NOT NULL CHECK (version > 0),
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    student_id text NOT NULL,
    rank int NOT NULL CHECK (rank > 0),
    display_name text NOT NULL,
    points int NOT NULL CHECK (points >= 0),
    possible_points int NOT NULL CHECK (possible_points > 0),
    elapsed_ms bigint NOT NULL CHECK (elapsed_ms >= 0),
    PRIMARY KEY (assessment_id, version, attempt_id)
);

CREATE INDEX assessment_leaderboard_entries_rank
    ON assessment_leaderboard_entries(assessment_id, version, rank, attempt_id);

CREATE INDEX assessment_leaderboard_entries_student
    ON assessment_leaderboard_entries(assessment_id, version, student_id);

CREATE TABLE assessment_student_reports (
    assessment_id uuid NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    version int NOT NULL CHECK (version > 0),
    attempt_id uuid NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    student_id text NOT NULL,
    status text NOT NULL CHECK (status IN ('published', 'withheld')),
    rank int,
    total_ranked int NOT NULL DEFAULT 0,
    report jsonb NOT NULL CHECK (jsonb_typeof(report) = 'object'),
    PRIMARY KEY (assessment_id, version, attempt_id)
);

CREATE INDEX assessment_student_reports_student
    ON assessment_student_reports(student_id, assessment_id, version);
