DROP TABLE IF EXISTS assessment_student_reports;
DROP TABLE IF EXISTS assessment_leaderboard_entries;
DROP TABLE IF EXISTS assessment_result_releases;
ALTER TABLE assessment_roster DROP COLUMN IF EXISTS display_alias;
ALTER TABLE assessments
    DROP COLUMN IF EXISTS results_published_at,
    DROP COLUMN IF EXISTS latest_result_version,
    DROP COLUMN IF EXISTS release_explanations;
