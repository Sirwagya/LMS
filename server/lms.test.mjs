import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";

test(
  "local LMS: roles, learning, persistence, files and simulated ledger",
  { timeout: 60000 },
  async () => {
    const dir = mkdtempSync(join(tmpdir(), "vedam-test-")),
      port = 31000 + Math.floor(Math.random() * 15000),
      base = `http://127.0.0.1:${port}`;
    let proc;
    async function start() {
      proc = spawn(process.execPath, ["server/index.mjs"], {
        env: { ...process.env, PORT: String(port), LMS_DATA_DIR: dir },
        stdio: ["ignore", "pipe", "pipe"],
      });
      let errors = "";
      proc.stderr.on("data", (d) => (errors += d));
      await new Promise((resolve, reject) => {
        const timer = setTimeout(
          () => reject(new Error("Server start timeout " + errors)),
          10000,
        );
        proc.stdout.once("data", () => {
          clearTimeout(timer);
          resolve();
        });
        proc.once("exit", (code) => {
          clearTimeout(timer);
          reject(new Error(`Server exited ${code}: ${errors}`));
        });
      });
    }
    async function stop() {
      const done = once(proc, "exit");
      proc.kill();
      await done;
    }
    async function req(path, method = "GET", data, cookie) {
      const r = await fetch(base + "/api" + path, {
        method,
        headers: {
          "Content-Type": "application/json",
          ...(cookie ? { Cookie: cookie } : {}),
        },
        body: data === undefined ? undefined : JSON.stringify(data),
      });
      const type = r.headers.get("content-type") || "";
      return {
        status: r.status,
        data: type.includes("application/json")
          ? await r.json()
          : await r.text(),
        cookie: r.headers.get("set-cookie")?.split(";")[0],
      };
    }
    async function login(role) {
      const r = await req("/login", "POST", {
        email: `${role}@vedam.local`,
        password: "VedamLocal123!",
      });
      assert.equal(r.status, 200);
      return r.cookie;
    }
    try {
      await start();
      assert.equal((await req("/bootstrap")).status, 401);
      assert.equal(
        (
          await req("/login", "POST", {
            email: "student@vedam.local",
            password: "wrong",
          })
        ).status,
        401,
      );
      const admin = await login("admin"),
        teacher = await login("teacher"),
        student = await login("student");
      assert.equal(
        (await req("/staff", "GET", undefined, student)).status,
        403,
      );
      assert.equal(
        (await req("/staff/users", "POST", {}, teacher)).status,
        403,
      );
      const codingProblem = (
        await req("/bootstrap", "GET", undefined, student)
      ).data.lecturesBySubject.E0460A.flatMap((l) => l.assignments).find(
        (a) => a.id === "coding-two-sum",
      ).problem;
      assert.equal(codingProblem.testCases, undefined);
      assert.equal(codingProblem.examples.length, 2);
      const solution =
        "class Solution:\n    def twoSum(self, nums: list[int], target: int) -> list[int]:\n        seen={}\n        for i,x in enumerate(nums):\n            if target-x in seen:\n                return [seen[target-x], i]\n            seen[x]=i";
      assert.equal(
        (
          await req("/coding/evaluate", "POST", {
            assignmentId: "coding-two-sum",
            language: "python",
            source: solution,
            action: "run",
          })
        ).status,
        401,
      );
      assert.equal(
        (
          await req(
            "/coding/evaluate",
            "POST",
            {
              assignmentId: "coding-two-sum",
              language: "python",
              source: solution,
              action: "run",
            },
            teacher,
          )
        ).status,
        403,
      );
      const run = await req(
        "/coding/evaluate",
        "POST",
        {
          assignmentId: "coding-two-sum",
          language: "python",
          source: solution,
          action: "run",
        },
        student,
      );
      assert.equal(run.data.verdict, "Accepted");
      assert.equal(run.data.total, 2);
      const wrong = await req(
        "/coding/evaluate",
        "POST",
        {
          assignmentId: "coding-two-sum",
          language: "python",
          source: "class Solution:\n    def twoSum(self, nums, target):\n        return [99, 100]",
          action: "submit",
        },
        student,
      );
      assert.equal(wrong.data.verdict, "Wrong Answer");
      const accepted = await req(
        "/coding/evaluate",
        "POST",
        {
          assignmentId: "coding-two-sum",
          language: "python",
          source: solution,
          action: "submit",
        },
        student,
      );
      assert.equal(accepted.data.verdict, "Accepted");
      assert.equal(accepted.data.passed, 5);
      assert.equal(accepted.data.results[2].input, undefined);
      assert.equal(
        (
          await req(
            "/coding/history?assignmentId=coding-two-sum",
            "GET",
            undefined,
            student,
          )
        ).data.length,
        2,
      );
      assert.equal(
        (
          await req(
            "/submissions",
            "POST",
            { assignmentId: "coding-two-sum", answer: "bypass" },
            student,
          )
        ).status,
        400,
      );
      assert.equal(
        (
          await req("/bootstrap", "GET", undefined, student)
        ).data.submissions.find((a) => a.assignment_id === "coding-two-sum")
          .score,
        "5/5",
      );
      const repeat = await req(
        "/coding/evaluate",
        "POST",
        {
          assignmentId: "coding-two-sum",
          language: "python",
          source: solution,
          action: "submit",
        },
        student,
      );
      assert.equal(repeat.data.verdict, "Accepted");
      const afterRepeat = (
        await req(
          "/coding/history?assignmentId=coding-two-sum",
          "GET",
          undefined,
          student,
        )
      ).data;
      assert.equal(afterRepeat.length, 3);
      assert.equal(afterRepeat[0].source, solution);
      assert.equal(afterRepeat[0].result.results[2].input, undefined);
      const registered = await req("/register", "POST", {
        name: "Test Student",
        email: "test@example.local",
        password: "TestPassword123!",
      });
      assert.equal(registered.status, 200);
      const fresh = registered.cookie;
      assert.equal(registered.data.user.role, "student");
      const uid = registered.data.user.id;
      let b = (await req("/bootstrap", "GET", undefined, fresh)).data;
      assert.equal(b.fees.length, 0);
      assert.deepEqual(b.contestSummary, {});
      assert.equal(b.semesterSubjects[3][0].attendedClasses, 0);
      assert.equal(
        (
          await req("/register", "POST", {
            name: "Other",
            email: "test@example.local",
            password: "TestPassword123!",
          })
        ).status,
        409,
      );
      assert.equal(
        (
          await req(
            "/staff/courses",
            "POST",
            {
              semester: 3,
              code: "TEST101",
              name: "Testing course",
              instructor: "Teacher",
              credits: 4,
            },
            teacher,
          )
        ).status,
        201,
      );
      const l = await req(
        "/staff/lectures",
        "POST",
        {
          course: "TEST101",
          title: "Testing lesson",
          summary: "Real sample notes",
        },
        teacher,
      );
      assert.equal(l.status, 201);
      assert.equal(
        (
          await req(
            "/staff/assignments",
            "POST",
            {
              lectureId: l.data.id,
              title: "Build a test",
              dueDate: "2026-12-31",
            },
            teacher,
          )
        ).status,
        201,
      );
      assert.equal(
        (
          await req(
            "/staff/assets",
            "POST",
            {
              lectureId: l.data.id,
              name: "test.txt",
              bytes: Buffer.from("hello materials").toString("base64"),
            },
            teacher,
          )
        ).status,
        201,
      );
      b = (await req("/bootstrap", "GET", undefined, fresh)).data;
      const lecture = b.lecturesBySubject.TEST101[0],
        a = lecture.assignments[0];
      const asset = await fetch(base + lecture.resources[0].url, {
        headers: { Cookie: fresh },
      });
      assert.equal(await asset.text(), "hello materials");
      assert.equal(
        (
          await req(
            "/submissions",
            "POST",
            {
              assignmentId: a.id,
              answer: "My solution",
              filename: "solution.txt",
              file: Buffer.from("my answer").toString("base64"),
            },
            fresh,
          )
        ).status,
        200,
      );
      b = (await req("/bootstrap", "GET", undefined, fresh)).data;
      const sub = b.submissions[0];
      assert.equal(sub.answer, "My solution");
      assert.equal(
        (await req("/submission-file/" + sub.id, "GET", undefined, student))
          .status,
        403,
      );
      assert.equal(
        (await req("/submission-file/" + sub.id, "GET", undefined, teacher))
          .data,
        "my answer",
      );
      assert.equal(
        (
          await req(
            "/staff/grade",
            "PUT",
            { id: sub.id, score: "21/20" },
            teacher,
          )
        ).status,
        400,
      );
      assert.equal(
        (
          await req(
            "/staff/grade",
            "PUT",
            { id: sub.id, score: "18/20", feedback: "Good work" },
            teacher,
          )
        ).status,
        200,
      );
      assert.equal(
        (
          await req(
            "/submissions",
            "POST",
            { assignmentId: a.id, answer: "Changed" },
            fresh,
          )
        ).status,
        409,
      );
      assert.equal(
        (
          await req(
            "/staff/attendance",
            "PUT",
            { userId: uid, course: "TEST101", attended: 8, total: 10 },
            teacher,
          )
        ).status,
        200,
      );
      assert.equal(
        (
          await req(
            "/staff/attendance",
            "PUT",
            { userId: uid, course: "TEST101", attended: 11, total: 10 },
            teacher,
          )
        ).status,
        400,
      );
      assert.equal(
        (
          await req(
            "/progress",
            "POST",
            { lectureId: l.data.id, complete: true },
            fresh,
          )
        ).status,
        200,
      );
      assert.equal(
        (
          await req(
            "/discussions",
            "POST",
            { lectureId: l.data.id, body: "How does this work?" },
            fresh,
          )
        ).status,
        201,
      );
      assert.equal(
        (
          await req(
            "/discussions?lecture=" + l.data.id,
            "GET",
            undefined,
            teacher,
          )
        ).data[0].name,
        "Test Student",
      );
      assert.equal(
        (
          await req(
            "/staff/notices",
            "POST",
            { title: "Test notice", desc: "Published", tag: "ACADEMIC" },
            teacher,
          )
        ).status,
        201,
      );
      assert.equal(
        (
          await req(
            "/staff/schedule",
            "POST",
            {
              course: "TEST101",
              date: "2026-12-01",
              from: "09:00",
              to: "10:00",
              room: "Room A",
              topic: "Testing",
            },
            teacher,
          )
        ).status,
        201,
      );
      assert.equal(
        (
          await req(
            "/staff/schedule",
            "POST",
            {
              course: "TEST101",
              date: "2026-12-01",
              from: "09:30",
              to: "10:30",
              room: "Room A",
              topic: "Overlap",
            },
            teacher,
          )
        ).status,
        409,
      );
      assert.equal(
        (
          await req(
            "/staff/fees",
            "POST",
            {
              userId: uid,
              description: "Test tuition",
              semester: 3,
              amount: 100,
              dueDate: "2026-12-01",
            },
            admin,
          )
        ).status,
        201,
      );
      b = (await req("/bootstrap", "GET", undefined, fresh)).data;
      const fee = b.fees[0];
      assert.equal(
        (await req("/payments", "POST", { feeId: fee.id }, student)).status,
        404,
      );
      assert.equal(
        (await req("/payments", "POST", { feeId: fee.id }, fresh)).status,
        200,
      );
      assert.equal(
        (await req("/payments", "POST", { feeId: fee.id }, fresh)).status,
        409,
      );
      assert.match(
        (await req("/receipts/" + fee.id, "GET", undefined, fresh)).data,
        /No real money collected/,
      );
      assert.equal(
        (await req("/receipts/" + fee.id, "GET", undefined, student)).status,
        404,
      );
      await stop();
      await start();
      b = (await req("/bootstrap", "GET", undefined, fresh)).data;
      assert.equal(b.submissions[0].score, "18/20");
      assert.equal(b.fees[0].status, "Paid");
      assert.equal(
        b.semesterSubjects[3].find((c) => c.code === "TEST101").attendedClasses,
        8,
      );
      assert.equal(b.contestSummary.TEST101.percent, 90);
      assert.ok(b.progress.includes(l.data.id));
      assert.equal(
        (
          await req(
            "/profile",
            "PUT",
            { profile: { name: "Updated student", platforms: [] } },
            fresh,
          )
        ).status,
        200,
      );
      assert.equal(
        (
          await req(
            "/password",
            "POST",
            { current: "TestPassword123!", next: "NewPassword123!" },
            fresh,
          )
        ).status,
        200,
      );
      assert.equal(
        (await req("/bootstrap", "GET", undefined, fresh)).status,
        401,
      );
      const next = await req("/login", "POST", {
        email: "test@example.local",
        password: "NewPassword123!",
      });
      assert.equal(next.status, 200);
      assert.equal((await req("/logout", "POST", {}, next.cookie)).status, 200);
      assert.equal(
        (await req("/bootstrap", "GET", undefined, next.cookie)).status,
        401,
      );
    } finally {
      if (proc?.exitCode === null) await stop();
      rmSync(dir, { recursive: true, force: true });
    }
  },
);
