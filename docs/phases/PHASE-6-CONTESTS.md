# LMS contests and integrity review

## Product decisions

Teachers create timed contests with MCQs and coding problems. Students take scored contests in the macOS app. Teachers can author and review from the existing staff portal. After publication, participants in the signed-in contest cohort can see the contest leaderboard and each student can open their own detailed report from the dashboard.

Replay records activity inside the contest, including code changes. Screen, webcam, microphone, clipboard contents, global keystrokes, and unrelated app activity stay outside collection.

## Authoring preview

Set `LMS_CONTEST_AUTHORING=true` for the local API to enable **Staff → Contests** and the student dashboard's contest schedule. This enables drafting, preview, publishing, cancellation, and cohort visibility. Also set `LMS_CONTEST_ATTEMPTS=true` for the API and worker to preview timed attempts in the macOS app. In this preview students can save MCQ answers and Python or Java drafts, switch devices, and get a submission receipt. The worker finalizes expired attempts. The grading preview adds scored contests, official coding submissions, and private grading status. Replay, reports, and rankings follow on their own branches.

### Grading preview

Set `LMS_CONTEST_GRADING=true` and configure `LMS_CONTEST_JUDGE_URL` and `LMS_CONTEST_JUDGE_TOKEN` for the API and worker before publishing a scored contest. The flag applies to new publications. Published preview contests retain their original mode. Keep the judge configuration available until live scored contests and their queues have drained.

Students choose **Submit code to judge** for each version they want scored. Finishing the attempt freezes MCQ answers and the already admitted coding submissions. Code drafts keep autosaving, and coding points come from official submissions. The worker retries infrastructure failures and marks attempts **Needs review** when a failed submission could change marks or tie-break time. Scores remain private until the results release workflow is available.

The LMS worker posts JSON with `language`, `source`, `entrypoint` (`stdin`), and `testCases` containing `input`, `expected`, and `hidden`. The endpoint receives a dedicated bearer secret and the official submission UUID in `Idempotency-Key`. It should deduplicate completed evaluations by that UUID and return HTTP 200 with only aggregate `verdict`, `passed`, and `total`. It can return a non-success response for infrastructure errors. Expected verdicts are Accepted, Wrong Answer, Compilation Error, Runtime Error, Time Limit Exceeded, Memory Limit Exceeded, and Output Limit Exceeded. LMS uses a 25-second request budget. Production requires a TLS endpoint. Loopback HTTP is available for development fixtures.

A production rollout requires provisioning and verifying an isolated execution service. The judge supervisor holds the judge token and launches disposable workers with no network, no LMS credentials, bounded resources, and cleanup on completion or timeout. Keep expected outputs inside the supervisor and limit each execution to its test input. Verify containment and failure recovery before enabling scored contests. The grading preview integration uses a mock judge, with actual code execution left to the isolated service.

### macOS lockdown preview

Starting a proctored contest clears the system clipboard after disclosure. The app blocks paste and drop and uses macOS presentation settings to restrict Command-Tab while the app is active. Leaving fullscreen or losing focus counts as one warning until the student returns. The server commits the third warning, freezes the last saved work and official coding submissions, and adds the attempt to faculty review. Browser events are observations and faculty decide the review outcome.

This recording-enabled preview uses AppKit presentation controls. Institution-managed Siri restrictions or Apple Automatic Assessment Configuration are required for OS-level prevention. Apple's assessment mode requires an approved entitlement and disables screen capture, so switching to it requires a separate capture policy and signed-device testing.

## Teacher workflow

1. In **Staff → Contests**, create a draft with a title, audience, instructions, opening time, closing time, attempt duration, and allowed resources. Show the timezone and preview exactly what students will see.
2. Add single-answer MCQs with choices, answer key, explanation, and points. Add Python or Java coding questions with starter code, examples, hidden cases, and points. Validate scoring, test cases, roster, schedule, and access accommodations before publishing.
3. Publishing freezes the roster, questions, scoring rules, and integrity policy in a contest revision. Corrections require an explicitly audited new revision or regrade. Preserve submitted work against its original revision. Cancelling records a reason.
4. During the contest, see starts, submissions, connectivity, and grading backlog. After it closes, review flagged attempts with synchronized evidence and a short reason field. Publish confirmed results, or withhold specific rows with a private **Under review** status. Record reasoned outcomes and allow appeals. Assign a different reviewer for appeals where staffing permits.
5. Keep editing and evidence access limited to the contest owner, assigned reviewers, and authorized administrators. Support a dry-run preview with sample student data.

## Student workflow

- Dashboard shows upcoming contests, eligibility, start time, and any saved attempt. Preflight shows rules, allowed resources, recorded activity, time accommodations, and connection status before **Start attempt**.
- The attempt workspace has a fixed timer, question navigation, a visible save indicator, and a review-and-submit action. MCQs support keyboard selection and flag-for-review. Coding uses the existing CodeMirror experience with examples, custom input, and submission status. Keep the app's light and dark styles and readable focus states.
- Starting creates one attempt and one active device session. A reload or temporary disconnect can resume it. The app keeps unsent edits in account-scoped temporary storage and clearly shows their save status. Timed attempts require server reachability to start, and the timer keeps running during a disconnect. The server submits the last acknowledged work at expiry. If a broad outage materially affected students, staff can grant an audited reschedule or retake.
- After submission, show a durable receipt, then **Grading**, **Under review**, or **Published** as appropriate. Published reports show score, rank, MCQ/coding breakdown, per-question points and verdicts, and the student's own submissions. Release MCQ explanations according to contest policy and keep hidden tests private. Explain review actions privately and provide an appeal entry point.

## Scoring and results

Start with single-answer MCQs and no negative marking. Score MCQs at finalization. For coding, use the best official submission received before the deadline, with all-or-nothing points per problem for the first release. Example runs carry zero points. Finalization freezes answer revisions and accepted submission IDs in one transaction. Official jobs admitted before the deadline may finish afterward. Persist verdicts and calculate results idempotently after all eligible jobs reach a terminal state. Tie-break by total points and then elapsed time to reach the final score, measured from server-recorded submissions. Exact ties share a rank. Regrades create a new result version and retain the prior publication audit trail.

The contest leaderboard is visible after release to the signed-in cohort. Show rank, contest display name, marks, and tie-break time. Allow a contest alias as the display name. Show the student's row even when it falls outside the first page. Publish a leaderboard version atomically with the corresponding report versions. Withheld attempts stay out of rankings until cleared. Keep email, source code, evidence, and review reasons private. The dashboard should read the latest released results from a dedicated API projection, with existing historical summaries preserved.

## Integrity controls and replay

A device outside the app's control can still be used to cheat. The goal is to reduce opportunities, preserve reliable evidence, and give reviewers enough context to decide fairly.

| Control | Decision |
| --- | --- |
| Trusted timing and submissions | The server sets the deadline from the contest window and individual duration, including accommodations. It enforces ownership, eligibility, deadlines, attempt state, revision checks, rate limits, and idempotency. A session takeover invalidates the previous lease and is recorded. |
| Question exposure | Release questions only after the server admits the attempt. Shuffle MCQ choices per attempt with a stable seed and retain the displayed order in replay. Isolate answer keys and hidden tests from student APIs, logs, and browser bundles. |
| macOS attempt mode | Use a dedicated contest window with fullscreen and explicit rules about external tools and paste. Capture app focus, fullscreen changes, and reconnects as review signals. Verify Apple Assessment Mode in a separate spike. It requires an entitlement and real-device validation. Gate strict OS lockdown behind those prerequisites. |
| Evidence | Record starts, question navigation, MCQ decisions, code snapshots and bounded deltas, run and submit actions, paste action and approximate size inside the editor, focus events, heartbeat gaps, reconnects, and session replacement. Mark authoritative server events and client observations distinctly. |
| Similarity and review | Compare code after normalizing formatting and subtracting shared starter scaffolding. Surface matched regions and supporting evidence. Put clusters, suspiciously large edits, and session anomalies in a review queue. Give students a way to report a concern privately. A paste, focus change, connection gap, or similarity score alone cannot disqualify a student. |
| Access and retention | Show collection and consequences before starting. Encrypt evidence, audit access, and enforce contest-level reviewer permissions. Draft retention is 30 days for replay and derived evidence, with an audited extension for open appeals. Confirm institutional retention and accommodations before rollout. |

Use an event timeline with a scrubber and periodic code checkpoints. Reviewers can jump to a submission, diff code between saved snapshots, or see the MCQ choice at a given time. The UI explicitly marks missing intervals. Batch client events with bounded per-attempt quotas, sequence numbers, and retry-safe batch IDs. Store server receipt times, validate event types, and show late-arriving or missing client batches. Storage integrity controls preserve what the server received and the trust level of each event.

See [Apple Automatic Assessment Configuration](https://developer.apple.com/documentation/automaticassessmentconfiguration) for device controls and entitlement requirements. LeetCode describes similarity detection followed by human review in its [contest fairness guidance](https://leetcode.com/discuss/post/5450505/Addressing-Contest-Fairness-and-Actions-on-Violations/).

## Architecture and contracts

- **PostgreSQL** owns contest revisions, participants, attempts and leases, answer revisions, coding submission jobs, versioned results, review actions, and release manifests. Use transactions and uniqueness constraints for attempt start, submission, score updates, and publishing. Partition evidence by contest with a deletion schedule. Start with bounded event payloads in PostgreSQL. Reassess object storage after measuring replay volume.
- **Go API** adds staff draft/publish/review/release routes, student contest list/start/resume/write/submit routes, an event batch route, a student report route, and a paginated cohort leaderboard route under `/api/v1`. Return server time and the personal deadline on start and resume. Write endpoints include the attempt session ID, an idempotency key, and an answer revision where applicable. Staff authorization combines role with contest assignment. Student authorization combines enrollment with attempt ownership.
- **Redis and worker** support rate limits and status updates. Persist official submissions and their queue state in the same PostgreSQL row. Worker leases and claim generations recover interrupted jobs and fence stale responses. Deduplicate admissions by client request ID and judge calls by official submission ID. Feed results back into the authoritative transaction and use SSE to deliver status hints. Reconnects refetch current state.
- **Isolated coding runner** reuses the grader request and verdict concepts. Production scored submissions require separate short-lived runner isolation with enforced CPU, memory, disk, process, output, and wall-clock limits, no network, and no production credentials. Mount hidden tests only for the trusted judge and redact them from logs and responses. Verify containment, cleanup, and cancellation before enabling scored coding contests. Keep the macOS local grader limited to development and practice.
- **Tauri and React** reuse the current authenticated transport and editor. Contest writes use their own retry path, independent of eventual offline sync. The server remains authoritative when a device returns online after the deadline.

For a first pilot, plan for one cohort and use 100 concurrent students as a load-test target until the expected class size is confirmed. Measure admission latency, save latency, official grading backlog, replay storage per attempt, and report publication latency during a realistic end-of-contest burst.

## Branches and acceptance gates

Use small, reviewable commits with the repo's configured author and committer. Start implementation branches from the latest merged prerequisite. Merge through an integration branch and keep the feature behind a flag until the applicable gates pass.

| Branch | Deliverable and gate |
| --- | --- |
| `codex/contest-core` | Contest schema, teacher authoring/scheduling, stable question revisions, enrollment, and role tests. |
| `codex/contest-attempts` | macOS student workspace, MCQ, saving/resume, timing and finalization. Prove duplicate starts and submits reconcile, deadline races agree, and stale device sessions cannot save. |
| `codex/contest-grading` | Coding UI, official judge queue, and versioned scores. Verify isolation, hidden-case confidentiality, retry/recovery, and that grading failures leave visible status. |
| `codex/contest-integrity` | Event collection, replay, similarity review, appeal, and retention. Test dropped or repeated batches, evidence permissions, and ordinary interruptions such as accessibility tools and connection loss. |
| `codex/contest-results` | Dashboard reports and cohort leaderboard. Verify ties, releases, regrades, withheld rows, stable pagination, and access by students outside the cohort. |
| `codex/contest-integration` | End-to-end pilot against Go, PostgreSQL, Redis, native macOS app, and isolated grader. Run concurrency and restart tests, check migration rollback on a disposable database, and evaluate signals with sample attempts before scored rollout. |

A production launch needs a supported isolated grader, a signed macOS build and device test environment, and institutional confirmation of evidence retention and access. Apple entitlement approval is a dependency for strict OS assessment mode. Before a pilot, confirm expected cohort size, contest rules, and supported accessibility accommodations.
