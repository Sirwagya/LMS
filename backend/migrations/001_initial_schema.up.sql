CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "btree_gist";

-- ============================================================================
-- 1. IAM & Campus Identity: Users, Campus Roles, Program Enrolments, Biometrics, Coding Handles
-- ============================================================================
CREATE TABLE users (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    email           TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    role            TEXT NOT NULL CHECK (role IN ('student', 'teacher', 'admin')),
    password_hash   TEXT NOT NULL,
    profile_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
    version         BIGINT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE coding_handles (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform_name   TEXT NOT NULL,
    handle          TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    profile_url     TEXT NOT NULL DEFAULT '',
    initial         TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coding_handles_user ON coding_handles(user_id, sort_order);

CREATE TABLE campuses (
    id              INT PRIMARY KEY,
    name            TEXT NOT NULL,
    organization_id INT NULL
);

CREATE TABLE programs (
    id              INT PRIMARY KEY,
    campus_id       INT NOT NULL REFERENCES campuses(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    label           TEXT NOT NULL UNIQUE
);

CREATE TABLE campus_person_roles (
    cpr_id          BIGINT PRIMARY KEY,
    user_id         TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    campus_id       INT NOT NULL REFERENCES campuses(id) ON DELETE RESTRICT,
    person_name     TEXT NOT NULL,
    email           TEXT NOT NULL
);

CREATE TABLE program_enrolments (
    id              BIGINT PRIMARY KEY,
    program_id      INT NOT NULL REFERENCES programs(id) ON DELETE RESTRICT,
    cpr_id          BIGINT NOT NULL REFERENCES campus_person_roles(cpr_id) ON DELETE CASCADE,
    enrolment_date  BIGINT NOT NULL
);
CREATE INDEX idx_program_enrolments_cpr ON program_enrolments(cpr_id);

CREATE TABLE user_face_biometrics (
    person_id       BIGINT PRIMARY KEY REFERENCES campus_person_roles(cpr_id) ON DELETE CASCADE,
    email           TEXT NOT NULL,
    has_face_data   BOOLEAN NOT NULL DEFAULT FALSE,
    face_json       TEXT NOT NULL DEFAULT '{}',
    embedding       DOUBLE PRECISION[] NOT NULL DEFAULT '{}',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- 2. Catalog & Offerings: Semesters, Courses, Offerings, Enrolments, Sessions, Lectures
-- ============================================================================
CREATE TABLE semesters (
    number              INT PRIMARY KEY CHECK (number > 0),
    label               TEXT NOT NULL,
    code_label          TEXT NOT NULL DEFAULT '',
    campus_id           INT NOT NULL DEFAULT 1,
    program_id          INT NOT NULL DEFAULT 1,
    program_title       TEXT NOT NULL DEFAULT 'BTech CSE AI 2025',
    program_label       TEXT NOT NULL DEFAULT 'BTECH_CSE_AI_25',
    start_epoch         BIGINT NOT NULL DEFAULT 0,
    end_epoch           BIGINT NOT NULL DEFAULT 0,
    required_attendance INT NOT NULL DEFAULT 75,
    is_current          BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE TABLE courses (
    code            TEXT PRIMARY KEY,
    semester_number INT NOT NULL REFERENCES semesters(number),
    name            TEXT NOT NULL,
    instructor      TEXT NOT NULL DEFAULT '',
    credits         INT NOT NULL CHECK (credits BETWEEN 1 AND 10),
    next_topic      TEXT NOT NULL DEFAULT '',
    default_att_pct TEXT NOT NULL DEFAULT '0%',
    skips_label     TEXT NOT NULL DEFAULT '0',
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_courses_semester ON courses(semester_number);

CREATE TABLE course_offerings (
    id                  BIGINT PRIMARY KEY,
    course_id           BIGINT NOT NULL,
    course_code         TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    semester_id         INT NOT NULL REFERENCES semesters(number),
    campus_id           INT NOT NULL DEFAULT 1,
    program_id          INT NOT NULL DEFAULT 1,
    label               TEXT NOT NULL,
    title               TEXT NOT NULL,
    batch               TEXT NOT NULL DEFAULT 'B1',
    credits             INT NOT NULL DEFAULT 4,
    description         TEXT NOT NULL DEFAULT '',
    icon_url            TEXT NOT NULL DEFAULT '',
    bg_color            TEXT NOT NULL DEFAULT '',
    enrolment_count     INT NOT NULL DEFAULT 0,
    class_session_count INT NOT NULL DEFAULT 0,
    required_attendance INT NOT NULL DEFAULT 75
);
CREATE INDEX idx_course_offerings_sem ON course_offerings(semester_id, id);

CREATE TABLE course_offering_enrolments (
    id                  BIGINT PRIMARY KEY,
    course_offering_id  BIGINT NOT NULL REFERENCES course_offerings(id) ON DELETE CASCADE,
    cpr_id              BIGINT NOT NULL REFERENCES campus_person_roles(cpr_id) ON DELETE CASCADE,
    enrolment_date      BIGINT NOT NULL,
    present_count       INT NOT NULL DEFAULT 0,
    absent_count        INT NOT NULL DEFAULT 0,
    late_count          INT NOT NULL DEFAULT 0,
    leave_count         INT NOT NULL DEFAULT 0,
    excused_count       INT NOT NULL DEFAULT 0,
    awaiting_count      INT NOT NULL DEFAULT 0,
    total_count         INT NOT NULL DEFAULT 0,
    UNIQUE (course_offering_id, cpr_id)
);
CREATE INDEX idx_coe_cpr ON course_offering_enrolments(cpr_id);

CREATE TABLE class_sessions (
    id                  BIGINT PRIMARY KEY,
    course_offering_id  BIGINT NOT NULL REFERENCES course_offerings(id) ON DELETE CASCADE,
    program_id          INT NOT NULL DEFAULT 1,
    semester_id         INT NOT NULL REFERENCES semesters(number),
    cpr_id              BIGINT NULL,
    start_time          BIGINT NOT NULL,
    end_time            BIGINT NOT NULL,
    title               TEXT NOT NULL,
    topic               TEXT NOT NULL,
    label               TEXT NOT NULL DEFAULT '',
    room                TEXT NULL,
    cs_created_at       BIGINT NOT NULL,
    cs_updated_at       BIGINT NOT NULL,
    CHECK (end_time >= start_time)
);
CREATE INDEX idx_class_sessions_sem_time ON class_sessions(semester_id, start_time);
CREATE INDEX idx_class_sessions_offering_time ON class_sessions(course_offering_id, start_time);

CREATE TABLE assets (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    filename        TEXT NOT NULL,
    mime_type       TEXT NOT NULL,
    byte_size       BIGINT NOT NULL CHECK (byte_size >= 0),
    storage_key     TEXT NOT NULL,
    sha256          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE lectures (
    id              BIGSERIAL PRIMARY KEY,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    lecture_code    TEXT NOT NULL,
    lecture_date    TEXT NOT NULL,
    month_label     TEXT NOT NULL DEFAULT '',
    unit_number     INT NOT NULL DEFAULT 1,
    title           TEXT NOT NULL,
    raw_title       TEXT NOT NULL,
    default_att     TEXT NOT NULL DEFAULT 'Awaiting' CHECK (default_att IN ('Present', 'Awaiting', 'Excused')),
    summary         TEXT NOT NULL DEFAULT '',
    slides          TEXT NOT NULL DEFAULT '',
    duration        TEXT NOT NULL DEFAULT '',
    recording_url   TEXT NULL,
    recording_asset TEXT NULL REFERENCES assets(id) ON DELETE SET NULL,
    sort_order      INT NOT NULL DEFAULT 0,
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_lectures_course ON lectures(course_code, sort_order);

CREATE TABLE lecture_resources (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    resource_type   TEXT NOT NULL DEFAULT 'File',
    size_label      TEXT NOT NULL DEFAULT '',
    url             TEXT NULL,
    asset_id        TEXT NULL REFERENCES assets(id) ON DELETE SET NULL,
    sort_order      INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_lecture_resources_lecture ON lecture_resources(lecture_id, sort_order);

CREATE TABLE schedule_slots (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    week_start      DATE NOT NULL,
    slot_date       DATE NOT NULL,
    time_from       TIME NOT NULL,
    time_to         TIME NOT NULL,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    room            TEXT NOT NULL,
    topic           TEXT NOT NULL,
    slot_type       TEXT NOT NULL DEFAULT 'Lecture' CHECK (slot_type IN ('Lecture', 'Lab', 'Workshop')),
    published       BOOLEAN NOT NULL DEFAULT TRUE,
    CHECK (time_to > time_from),
    EXCLUDE USING gist (
        slot_date WITH =,
        tsrange((slot_date + time_from), (slot_date + time_to), '[)') WITH &&
    )
);
CREATE INDEX idx_schedule_slots_date ON schedule_slots(slot_date, time_from);

CREATE TABLE notices (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    placement       TEXT NOT NULL CHECK (placement IN ('dashboard', 'drawer')),
    tag             TEXT NOT NULL,
    tag_bg          TEXT NOT NULL DEFAULT '',
    time_label      TEXT NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    action_label    TEXT NULL,
    action_route    TEXT NULL,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_notices_placement ON notices(placement, sort_order ASC, created_at DESC);

-- ============================================================================
-- 3. Learning: Attendance, Progress, Discussions
-- ============================================================================
CREATE TABLE attendance_summaries (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    attended        INT NOT NULL DEFAULT 0 CHECK (attended >= 0),
    total           INT NOT NULL DEFAULT 0 CHECK (total >= attended),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, course_code)
);

CREATE TABLE lecture_progress (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    completed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, lecture_id)
);

CREATE TABLE discussions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_discussions_lecture ON discussions(lecture_id, created_at ASC);

-- ============================================================================
-- 4. Assessment: Assignments, Coding Problems, Test Cases, Submissions, Contests
-- ============================================================================
CREATE TABLE assignments (
    id              TEXT PRIMARY KEY,
    lecture_id      BIGINT NOT NULL REFERENCES lectures(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'written' CHECK (kind IN ('written', 'coding')),
    title           TEXT NOT NULL,
    due_date        TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_assignments_lecture ON assignments(lecture_id);
CREATE INDEX idx_assignments_course ON assignments(course_code);

CREATE TABLE coding_problems (
    assignment_id   TEXT PRIMARY KEY REFERENCES assignments(id) ON DELETE CASCADE,
    entrypoint      TEXT NOT NULL CHECK (entrypoint IN ('twoSum', 'isValid', 'solve', 'stdin')),
    description     TEXT NOT NULL,
    input_format    TEXT NOT NULL,
    output_format   TEXT NOT NULL,
    constraints     TEXT NOT NULL,
    difficulty      TEXT NOT NULL CHECK (difficulty IN ('Easy', 'Medium', 'Hard')),
    starter_code    JSONB NOT NULL
);

CREATE TABLE coding_test_cases (
    id              BIGSERIAL PRIMARY KEY,
    assignment_id   TEXT NOT NULL REFERENCES coding_problems(assignment_id) ON DELETE CASCADE,
    ordinal         INT NOT NULL,
    input_data      TEXT NOT NULL,
    expected_output TEXT NOT NULL,
    is_hidden       BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (assignment_id, ordinal)
);

CREATE TABLE submissions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assignment_id   TEXT NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    answer          TEXT NOT NULL DEFAULT '',
    filename        TEXT NOT NULL DEFAULT '',
    asset_id        TEXT NULL REFERENCES assets(id) ON DELETE SET NULL,
    score           TEXT NULL,
    feedback        TEXT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, assignment_id)
);
CREATE INDEX idx_submissions_user ON submissions(user_id);

CREATE TABLE coding_attempts (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assignment_id   TEXT NOT NULL REFERENCES assignments(id) ON DELETE CASCADE,
    language        TEXT NOT NULL CHECK (language IN ('python', 'java')),
    source          TEXT NOT NULL,
    verdict         TEXT NOT NULL,
    passed          INT NOT NULL CHECK (passed >= 0),
    total           INT NOT NULL CHECK (total >= passed),
    result_json     JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_coding_attempts_user_assignment
    ON coding_attempts(user_id, assignment_id, created_at DESC);

CREATE TABLE contest_summaries (
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    percent         INT NOT NULL CHECK (percent BETWEEN 0 AND 100),
    detail          TEXT NOT NULL DEFAULT '',
    completed       INT NULL,
    total           INT NULL,
    best_rank       TEXT NULL,
    PRIMARY KEY (user_id, course_code)
);

CREATE TABLE contest_attempts (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    label           TEXT NOT NULL,
    title           TEXT NOT NULL,
    contest_date    TEXT NOT NULL,
    score           TEXT NOT NULL,
    percent         INT NOT NULL CHECK (percent BETWEEN 0 AND 100),
    rank_label      TEXT NOT NULL,
    ordinal         INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_contest_attempts_user_course ON contest_attempts(user_id, course_code, ordinal);

CREATE TABLE recent_contests (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    course_code     TEXT NOT NULL REFERENCES courses(code) ON DELETE CASCADE,
    contest_date    TEXT NOT NULL,
    score           TEXT NOT NULL,
    percent         INT NOT NULL CHECK (percent BETWEEN 0 AND 100),
    rank_label      TEXT NOT NULL,
    status          TEXT NOT NULL CHECK (status IN ('Completed', 'Reviewed')),
    ordinal         INT NOT NULL DEFAULT 0
);
CREATE INDEX idx_recent_contests_user ON recent_contests(user_id, ordinal);

-- ============================================================================
-- 5. Finance: Fee Invoices & Idempotent Payments
-- ============================================================================
CREATE TABLE fee_invoices (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    invoice_no      TEXT NOT NULL,
    description     TEXT NOT NULL,
    semester        TEXT NOT NULL,
    amount          BIGINT NOT NULL CHECK (amount > 0),
    due_date        TEXT NOT NULL,
    paid_date       TEXT NULL,
    status          TEXT NOT NULL CHECK (status IN ('Paid', 'Overdue', 'Pending')),
    receipt_ready   BOOLEAN NOT NULL DEFAULT FALSE,
    simulated       BOOLEAN NOT NULL DEFAULT TRUE,
    version         BIGINT NOT NULL DEFAULT 1,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_fee_invoices_user ON fee_invoices(user_id);

CREATE TABLE payments (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fee_id          TEXT NOT NULL UNIQUE REFERENCES fee_invoices(id) ON DELETE RESTRICT,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- 6. Transactional Outbox & Offline Sync Changelog
-- ============================================================================
CREATE TABLE outbox_events (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    aggregate_id    TEXT NOT NULL,
    topic           TEXT NOT NULL,
    payload         JSONB NOT NULL,
    published_at    TIMESTAMPTZ NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_outbox_unpublished ON outbox_events(created_at ASC) WHERE published_at IS NULL;

CREATE TABLE sync_changelog (
    seq             BIGSERIAL PRIMARY KEY,
    scope           TEXT NOT NULL,
    target_user_id  TEXT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL,
    entity_id       TEXT NOT NULL,
    op              TEXT NOT NULL CHECK (op IN ('upsert', 'delete')),
    payload         JSONB NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sync_changelog_user_seq ON sync_changelog(target_user_id, seq);

CREATE TABLE processed_mutations (
    idempotency_key TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    response_json   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
