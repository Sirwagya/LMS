import { spawn, spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const directory = mkdtempSync(join(tmpdir(), "vedam-lms-"));
process.on("exit", () => rmSync(directory, { recursive: true, force: true }));

const build = spawnSync("go", ["build", "-o", directory + "/", "./cmd/api", "./cmd/worker"], {
  cwd: "backend",
  stdio: "inherit",
});
if (build.error) console.error(build.error.message);
if (build.status !== 0) process.exit(build.status || 1);

const children = [
  spawn(join(directory, "api"), { stdio: "inherit" }),
  spawn(join(directory, "worker"), { stdio: "inherit" }),
];
if (process.argv.includes("--dev")) {
  children.push(spawn(
    process.execPath,
    ["node_modules/vite/bin/vite.js", "--host", "127.0.0.1"],
    { stdio: "inherit" },
  ));
}

let stopping = false;
function stop(code) {
  if (stopping) return;
  stopping = true;
  process.exitCode = code;
  for (const child of children) child.kill("SIGTERM");
  setTimeout(() => {
    for (const child of children) child.kill("SIGKILL");
  }, 8000).unref();
}
for (const child of children) {
  child.on("error", (error) => { console.error(error.message); stop(1); });
  child.on("exit", (code) => stop(code ?? 1));
}
process.on("SIGINT", () => stop(0));
process.on("SIGTERM", () => stop(0));
