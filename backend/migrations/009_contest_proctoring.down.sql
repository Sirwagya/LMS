ALTER TABLE assessment_attempt_events
    DROP CONSTRAINT IF EXISTS assessment_attempt_events_event_type_check;

DELETE FROM assessment_attempt_events
WHERE event_type IN (
    'app.opened',
    'proctor.camera_lost',
    'proctor.mic_lost',
    'proctor.screen_lost'
);

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
        'client.reconnect'
    ));

DROP TABLE IF EXISTS assessment_proctor_media;
