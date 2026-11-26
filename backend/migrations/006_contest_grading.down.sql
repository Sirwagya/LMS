DROP TABLE assessment_attempt_results;
DROP TABLE assessment_coding_submissions;
DROP INDEX assessment_attempts_awaiting_score;
ALTER TABLE assessment_attempts
    DROP COLUMN choice_saved_at,
    DROP COLUMN score_computed_at,
    DROP COLUMN scoring_enabled;
ALTER TABLE assessments DROP COLUMN official_scoring;
