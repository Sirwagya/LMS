DELETE FROM assessment_attempt_events WHERE event_type = 'lockdown.warning';
UPDATE assessment_attempts SET finish_reason = 'manual' WHERE finish_reason = 'lockdown';
ALTER TABLE assessment_attempts
    DROP CONSTRAINT assessment_attempts_finish_reason_check,
    ADD CONSTRAINT assessment_attempts_finish_reason_check CHECK (finish_reason IN ('manual', 'expired')),
    DROP COLUMN lockdown_warnings;

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
