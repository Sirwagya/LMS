import { spawn, execFile, execFileSync } from "node:child_process";
import { realpathSync, existsSync } from "node:fs";
import { readFile, readdir, lstat } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

export function runtimeConfig() {
  if (process.platform !== "darwin")
    throw new Error(
      "This local runner requires macOS sandbox-exec. No unrestricted fallback is enabled.",
    );
  let python = realpathSync(
    process.env.LMS_PYTHON || "/opt/homebrew/bin/python3",
  );
  const prefix = execFileSync(
    python,
    ["-I", "-S", "-c", "import sys; print(sys.prefix)"],
    { encoding: "utf8" },
  ).trim();
  const frameworkPython = join(
    prefix,
    "Resources/Python.app/Contents/MacOS/Python",
  );
  if (existsSync(frameworkPython)) python = frameworkPython;
  const javaHome =
    process.env.JAVA_HOME ||
    execFileSync("/usr/libexec/java_home", [], { encoding: "utf8" }).trim();
  return {
    python,
    java: join(javaHome, "bin/java"),
    javac: join(javaHome, "bin/javac"),
  };
}
export function profileFor(directory, runtime) {
  return `(version 1)
(deny default)
(allow file-read-metadata)
(allow file-map-executable)
(allow file-read* (literal "/") (subpath "/System") (subpath "/usr") (subpath "/Library/Java") (subpath "/opt/homebrew") (subpath "/private/preboot") (subpath "/private/var/db/dyld"))
(allow file-read* file-write* (subpath ${JSON.stringify(directory)}))
(allow file-read* file-write* (literal "/dev/null"))
(allow file-read* (literal "/dev/random") (literal "/dev/urandom"))
(allow process-exec ${[runtime.python, runtime.java, runtime.javac].map((p) => `(literal ${JSON.stringify(p)})`).join(" ")})
(allow sysctl-read)
(allow signal (target self))
`;
}
export const launcherSource = () =>
  readFile(fileURLToPath(new URL("./launcher.py", import.meta.url)), "utf8");

// Each child has a fresh directory, clean environment and OS-enforced restrictions.
export function execute({
  runtime,
  directory,
  command,
  args,
  stdin = "",
  phase = "run",
}) {
  return new Promise((resolve) => {
    const start = performance.now();
    let stdout = "",
      stderr = "",
      reason = null,
      settled = false;
    const child = spawn(
      "/usr/bin/sandbox-exec",
      [
        "-f",
        join(directory, "profile.sb"),
        runtime.python,
        "-I",
        "-S",
        join(directory, "launcher.py"),
        phase,
        command,
        ...args,
      ],
      {
        cwd: directory,
        detached: true,
        env: {
          PATH: "/usr/bin:/bin",
          HOME: directory,
          TMPDIR: directory,
          LANG: "en_US.UTF-8",
        },
        stdio: ["pipe", "pipe", "pipe"],
      },
    );
    const stop = (why) => {
      if (settled) return;
      reason = why;
      try {
        process.kill(-child.pid, "SIGKILL");
      } catch {
        /* Already exited. */
      }
    };
    const limit = setTimeout(
      () => stop("Time Limit Exceeded"),
      phase === "compile" ? 10000 : 3000,
    );
    let checking = false;
    const disk = setInterval(async () => {
      if (checking || settled) return;
      checking = true;
      try {
        execFile(
          "/bin/ps",
          ["-o", "rss=", "-p", String(child.pid)],
          { timeout: 500 },
          (error, out) => {
            if (!error && Number(out.trim()) > 384 * 1024)
              stop("Memory Limit Exceeded");
          },
        );
        let count = 0,
          size = 0;
        const pending = [directory];
        while (pending.length) {
          const parent = pending.pop();
          for (const name of await readdir(parent)) {
            const entry = join(parent, name),
              meta = await lstat(entry);
            count++;
            size += meta.size;
            if (count > 60 || size > 5 * 1024 * 1024) {
              stop("File Limit Exceeded");
              pending.length = 0;
              break;
            }
            if (meta.isDirectory()) pending.push(entry);
          }
        }
      } catch {
        /* Directory is being cleaned. */
      } finally {
        checking = false;
      }
    }, 100);
    const finish = (code, signal) => {
      if (settled) return;
      settled = true;
      clearTimeout(limit);
      clearInterval(disk);
      resolve({
        code,
        stdout,
        stderr,
        status:
          reason ||
          (signal === "SIGXCPU"
            ? "Time Limit Exceeded"
            : code === 0
              ? "Executed"
              : "Runtime Error"),
        durationMs: Math.round(performance.now() - start),
      });
    };
    for (const [stream, key] of [
      [child.stdout, "stdout"],
      [child.stderr, "stderr"],
    ])
      stream.on("data", (b) => {
        if (key === "stdout") stdout += b.toString();
        else stderr += b.toString();
        if (Buffer.byteLength(stdout) + Buffer.byteLength(stderr) > 65536) {
          stdout = stdout.slice(0, 32000);
          stderr = stderr.slice(0, 32000);
          stop("Output Limit Exceeded");
        }
      });
    child.on("error", (e) => {
      stderr = e.message;
      finish(-1);
    });
    child.on("close", finish);
    child.stdin.on("error", () => {});
    child.stdin.end(stdin);
  });
}
