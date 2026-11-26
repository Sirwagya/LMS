CREATE TABLE assessment_proctor_media (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    attempt_id UUID NOT NULL REFERENCES assessment_attempts(id) ON DELETE CASCADE,
    assessment_id UUID NOT NULL REFERENCES assessments(id) ON DELETE CASCADE,
    student_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('camera_audio', 'screen_snapshot', 'screen_video')),
    seq INTEGER NOT NULL CHECK (seq >= 1),
    content_type TEXT NOT NULL,
    storage_key TEXT NOT NULL UNIQUE,
    byte_size INTEGER NOT NULL CHECK (byte_size > 0),
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    captured_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (attempt_id, kind, seq)
);

CREATE INDEX assessment_proctor_media_attempt_kind_seq
    ON assessment_proctor_media (attempt_id, kind, seq);

CREATE INDEX assessment_proctor_media_assessment_created
    ON assessment_proctor_media (assessment_id, created_at);

ALTER TABLE assessment_attempt_events
    DROP CONSTRAINT IF EXISTS assessment_attempt_events_event_type_check;

ALTER TABLE assessment_attempt_events
    ADD CONSTRAINT assessment_attempt_events_event_type_check CHECK (event_type IN (
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
        'client.reconnect',
        'app.opened',
        'proctor.camera_lost',
        'proctor.mic_lost',
        'proctor.screen_lost'
    ));
