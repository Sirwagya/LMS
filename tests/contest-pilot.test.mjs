import { test } from "node:test";
import assert from "node:assert/strict";
import http from "node:http";
import { randomUUID } from "node:crypto";
import { setTimeout as delay } from "node:timers/promises";
import { isolatedGoServer } from "./go-server.mjs";

const HIDDEN_SECRET = "HIDDEN_NODE_PILOT_SECRET";
const JUDGE_TOKEN = "pilot-mock-judge-token-0123456789abcdef";

function startMockJudge() {
  const calls = [];
  const server = http.createServer((req, res) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => {
      try {
        assert.equal(req.method, "POST");
        assert.equal(req.headers.authorization, `Bearer ${JUDGE_TOKEN}`);
        const payload = JSON.parse(Buffer.concat(chunks).toString("utf8"));
        assert.equal(payload.entrypoint, "stdin");
        assert.equal(payload.testCases.length, 2);
        assert.equal(payload.testCases[0].hidden, false);
        assert.equal(payload.testCases[1].hidden, true);
        assert.equal(payload.testCases[1].expected, HIDDEN_SECRET);
        calls.push({
          idempotencyKey: req.headers["idempotency-key"],
          language: payload.language,
          source: payload.source,
        });
        res.writeHead(200, { "Content-Type": "application/json" });
        res.end(
          JSON.stringify({
            verdict: "Accepted",
            passed: 2,
            total: 2,
            diagnostic: HIDDEN_SECRET,
          }),
        );
      } catch (err) {
        console.error("mock judge error:", err.message);
        res.writeHead(400, { "Content-Type": "text/plain" });
        res.end(err.message);
      }
    });
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      resolve({
        url: `http://127.0.0.1:${address.port}/grade`,
        calls,
        close: () => new Promise((done) => server.close(done)),
      });
    });
  });
}

test(
  "contest pilot: flags, authoring, native transport, telemetry, worker grading, and results",
  { timeout: 60000 },
  async () => {
    const judge = await startMockJudge();
    const server = await isolatedGoServer({
      worker: true,
      env: {
        LMS_CONTEST_AUTHORING: "true",
        LMS_CONTEST_ATTEMPTS: "true",
        LMS_CONTEST_GRADING: "true",
        LMS_CONTEST_INTEGRITY: "true",
        LMS_CONTEST_RESULTS: "true",
        LMS_CONTEST_JUDGE_URL: judge.url,
        LMS_CONTEST_JUDGE_TOKEN: JUDGE_TOKEN,
      },
    });

    async function apiCall(base, path, { method = "GET", body, token, headers = {} } = {}) {
      const payload = body === undefined ? undefined : JSON.stringify(body);
      const target = new URL(path, base);
      return new Promise((resolve, reject) => {
        const req = http.request(
          target,
          {
            method,
            headers: {
              ...(payload === undefined
                ? {}
                : {
                    "Content-Type": "application/json",
                    "Content-Length": Buffer.byteLength(payload),
                  }),
              ...(token ? { Authorization: `Bearer ${token}` } : {}),
              ...headers,
            },
          },
          (res) => {
            const chunks = [];
            res.on("data", (chunk) => chunks.push(chunk));
            res.on("end", () => {
              const raw = Buffer.concat(chunks).toString("utf8");
              const contentType = String(res.headers["content-type"] || "");
              resolve({
                status: res.statusCode ?? 0,
                raw,
                data: contentType.includes("application/json") && raw ? JSON.parse(raw) : raw,
              });
            });
          },
        );
        req.on("error", reject);
        if (payload !== undefined) req.write(payload);
        req.end();
      });
    }

    async function nativeLogin(base, role) {
      const res = await apiCall(base, "/api/v1/login", {
        method: "POST",
        headers: { "X-LMS-Auth": "bearer" },
        body: { email: `${role}@vedam.local`, password: "VedamLocal123!" },
      });
      assert.equal(res.status, 200, res.raw);
      assert.ok(res.data.token, "expected bearer token from native login");
      return res.data;
    }

    try {
      const base = await server.start();
      const teacher = await nativeLogin(base, "teacher");
      const student = await nativeLogin(base, "student");

      const statusRes = await apiCall(base, "/api/contests/status", {
        token: student.token,
      });
      assert.equal(statusRes.status, 200, statusRes.raw);
      assert.deepEqual(statusRes.data, {
        enabled: true,
        attemptsEnabled: true,
        integrityEnabled: true,
        resultsEnabled: true,
      });

      const opensAtMs = Date.now() + 1200;
      const closesAtMs = opensAtMs + 60 * 60 * 1000;
      const createRes = await apiCall(base, "/api/staff/contests", {
        method: "POST",
        token: teacher.token,
        body: {
          draft: {
            title: "Node Pilot Contest",
            instructions: "Complete both questions in the native macOS workspace.",
            opensAt: new Date(opensAtMs).toISOString(),
            closesAt: new Date(closesAtMs).toISOString(),
            durationMinutes: 30,
            studentIds: [student.user.id],
            questions: [
              {
                kind: "mcq",
                prompt: "What is 2 + 2?",
                points: 5,
                choices: ["4", "5"],
                correctChoice: 0,
                explanation: "2 + 2 equals 4.",
              },
              {
                kind: "coding",
                prompt: "Echo stdin",
                points: 15,
                testCases: [
                  { input: "hello", expected: "hello", hidden: false },
                  { input: "secret", expected: HIDDEN_SECRET, hidden: true },
                ],
              },
            ],
          },
        },
      });
      assert.equal(createRes.status, 201, createRes.raw);
      const contestId = createRes.data.id;

      const publishRes = await apiCall(base, `/api/staff/contests/${contestId}/publish`, {
        method: "POST",
        token: teacher.token,
        body: { revision: createRes.data.revision },
      });
      assert.equal(publishRes.status, 200, publishRes.raw);

      const waitMs = opensAtMs - Date.now() + 150;
      if (waitMs > 0) await delay(waitMs);

      const deviceId = randomUUID();
      const nativeHeaders = { "X-LMS-Contest-Client": "macos" };

      // Browser-style requests must be rejected with 403.
      const missingClientRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        body: { action: "state", deviceId },
      });
      assert.equal(missingClientRes.status, 403, missingClientRes.raw);

      const withOriginRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: { ...nativeHeaders, Origin: base },
        body: { action: "state", deviceId },
      });
      assert.equal(withOriginRes.status, 403, withOriginRes.raw);

      const browserEventsRes = await apiCall(base, `/api/v1/contests/${contestId}/events`, {
        method: "POST",
        token: student.token,
        headers: { ...nativeHeaders, Origin: base },
        body: {
          attemptId: randomUUID(),
          deviceId,
          batchId: randomUUID(),
          events: [{ seq: 1, eventType: "nav.question", questionIndex: 0, payload: {} }],
        },
      });
      assert.equal(browserEventsRes.status, 403, browserEventsRes.raw);

      // Native macOS bearer transport: state -> start -> save -> events -> code -> submit -> grading.
      const stateRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: { action: "state", deviceId },
      });
      assert.equal(stateRes.status, 200, stateRes.raw);
      assert.equal(stateRes.data.state, "ready");

      const startRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: { action: "start", deviceId },
      });
      assert.equal(startRes.status, 200, startRes.raw);
      assert.equal(startRes.data.state, "active");
      assert.equal(startRes.data.officialCoding, true);
      assert.equal(startRes.data.lease, 1);
      assert.equal(startRes.data.questions.length, 2);
      assert.equal(startRes.raw.includes(HIDDEN_SECRET), false);

      const attemptId = startRes.data.attemptId;
      const correctChoiceIdx = startRes.data.questions[0].choices.indexOf("4");
      assert.notEqual(correctChoiceIdx, -1);

      const saveRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: {
          action: "save",
          deviceId,
          lease: startRes.data.lease,
          revision: startRes.data.revision,
          answers: { 0: { choice: correctChoiceIdx } },
        },
      });
      assert.equal(saveRes.status, 200, saveRes.raw);
      assert.equal(saveRes.data.revision, 1);

      const eventsRes = await apiCall(base, `/api/v1/contests/${contestId}/events`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: {
          attemptId,
          deviceId,
          batchId: randomUUID(),
          events: [
            { seq: 1, eventType: "nav.question", questionIndex: 0, payload: {} },
            { seq: 2, eventType: "mcq.selected", questionIndex: 0, payload: { choiceIndex: correctChoiceIdx } },
            {
              seq: 3,
              eventType: "code.snapshot",
              questionIndex: 1,
              payload: { language: "python", charCount: 14, source: "print(input())" },
            },
          ],
        },
      });
      assert.equal(eventsRes.status, 200, eventsRes.raw);
      assert.equal(eventsRes.data.accepted, 3);
      assert.equal(eventsRes.data.duplicate, false);
      assert.equal(eventsRes.data.highestSeq, 3);
      assert.deepEqual(eventsRes.data.missingSeqs ?? [], []);

      const codeRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: {
          action: "code",
          deviceId,
          lease: saveRes.data.lease,
          requestId: randomUUID(),
          questionIndex: 1,
          language: "python",
          source: "print(input())",
        },
      });
      assert.equal(codeRes.status, 200, codeRes.raw);
      assert.ok(codeRes.data.codingSubmissionId);
      assert.equal(codeRes.raw.includes(HIDDEN_SECRET), false);

      const submitRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
        method: "POST",
        token: student.token,
        headers: nativeHeaders,
        body: {
          action: "submit",
          deviceId,
          lease: saveRes.data.lease,
          revision: saveRes.data.revision,
          answers: {
            0: { choice: correctChoiceIdx },
            1: { language: "python", source: "print(input())" },
          },
        },
      });
      assert.equal(submitRes.status, 200, submitRes.raw);
      assert.equal(submitRes.data.finishReason, "manual");

      let gradingView;
      for (let attempt = 0; attempt < 40; attempt++) {
        const pollRes = await apiCall(base, `/api/v1/contests/${contestId}/attempt`, {
          method: "POST",
          token: student.token,
          headers: nativeHeaders,
          body: { action: "grading", deviceId },
        });
        assert.equal(pollRes.status, 200, pollRes.raw);
        assert.equal(pollRes.raw.includes(HIDDEN_SECRET), false);
        gradingView = pollRes.data;
        if (gradingView.gradingStatus === "awaiting_release") break;
        await delay(250);
      }
      assert.equal(
        gradingView.gradingStatus,
        "awaiting_release",
        `unexpected gradingView=${JSON.stringify(gradingView)} judgeCalls=${judge.calls.length}`,
      );
      assert.equal(gradingView.codingSubmissions[0].status, "accepted");
      assert.equal(judge.calls.length, 1);

      // Publish results -> student sets alias and fetches summary, report, leaderboard.
      const releaseRes = await apiCall(base, `/api/staff/contests/${contestId}/results/publish`, {
        method: "POST",
        token: teacher.token,
        body: { reason: "Pilot release", releaseExplanations: true },
      });
      assert.equal(releaseRes.status, 200, releaseRes.raw);
      assert.equal(releaseRes.data.version, 1);

      const aliasRes = await apiCall(base, `/api/contests/${contestId}/alias`, {
        method: "PUT",
        token: student.token,
        body: { alias: "PilotAce" },
      });
      assert.equal(aliasRes.status, 200, aliasRes.raw);
      assert.equal(aliasRes.data.alias, "PilotAce");

      const resultsSummaryRes = await apiCall(base, "/api/contests/results", {
        token: student.token,
      });
      assert.equal(resultsSummaryRes.status, 200, resultsSummaryRes.raw);
      assert.equal(resultsSummaryRes.raw.includes(HIDDEN_SECRET), false);
      const summary = resultsSummaryRes.data.items.find((item) => item.assessmentId === contestId);
      assert.ok(summary, "expected contest in /api/contests/results");
      assert.equal(summary.status, "published");
      assert.equal(summary.points, 20);
      assert.equal(summary.possiblePoints, 20);
      assert.equal(summary.rank, 1);

      const reportRes = await apiCall(base, `/api/contests/${contestId}/report`, {
        token: student.token,
      });
      assert.equal(reportRes.status, 200, reportRes.raw);
      assert.equal(reportRes.raw.includes(HIDDEN_SECRET), false);
      assert.equal(reportRes.data.points, 20);
      assert.equal(reportRes.data.mcqPoints, 5);
      assert.equal(reportRes.data.codingPoints, 15);
      assert.equal(reportRes.data.rank, 1);
      assert.equal(reportRes.data.breakdown[0].explanation, "2 + 2 equals 4.");

      const leaderboardRes = await apiCall(base, `/api/contests/${contestId}/leaderboard?page=0`, {
        token: student.token,
      });
      assert.equal(leaderboardRes.status, 200, leaderboardRes.raw);
      assert.equal(leaderboardRes.raw.includes(HIDDEN_SECRET), false);
      assert.equal(leaderboardRes.data.totalRanked, 1);
      assert.equal(leaderboardRes.data.items[0].displayName, "PilotAce");
      assert.equal(leaderboardRes.data.items[0].points, 20);
      assert.equal(leaderboardRes.data.items[0].rank, 1);
      assert.equal(leaderboardRes.data.items[0].isSelf, true);
    } finally {
      await server.close();
      await judge.close();
    }
  },
);
