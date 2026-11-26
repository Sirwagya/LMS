import { execFile, spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, rm } from "node:fs/promises";
import { join, resolve } from "node:path";
import { promisify } from "node:util";
import { setTimeout as delay } from "node:timers/promises";
import { randomBytes } from "node:crypto";

const exec = promisify(execFile);
const defaultDB = "postgres://lms:lms_local_dev@127.0.0.1:5432/lms_test?sslmode=disable";

async function stop(proc) {
  if (!proc || proc.exitCode !== null || proc.signalCode !== null) return;
  const exited = once(proc, "exit");
  proc.kill();
  await exited;
}

function startAndWait(binPath, env, label, pattern, parseMatch) {
  const proc = spawn(binPath, [], { env, stdio: ["ignore", "ignore", "pipe"] });
  const ready = new Promise((resolveReady, rejectReady) => {
    let logs = "";
    let settled = false;
    const finish = (fn, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      proc.stderr.off("data", onData);
      fn(value);
    };
    const timer = setTimeout(
      () => finish(rejectReady, new Error(`${label} startup timed out\n${logs}`)),
      10000,
    );
    const onData = (data) => {
      logs += data;
      const match = logs.match(pattern);
      if (match) finish(resolveReady, parseMatch(match));
    };
    proc.once("error", (error) => finish(rejectReady, error));
    proc.once("exit", (code) => finish(rejectReady, new Error(`${label} exited ${code}\n${logs}`)));
    proc.stderr.on("data", onData);
  });
  return { proc, ready };
}

export async function isolatedGoServer(options = {}) {
  const source = process.env.TEST_DATABASE_URL || defaultDB;
  const schema = "lms_http_" + randomBytes(10).toString("hex");
  const db = new URL(source);
  db.searchParams.set("search_path", `${schema},public`);
  const dir = await mkdtemp("/tmp/lms-http-");
  const socket = join(dir, "redis.sock");
  const env = {
    ...process.env,
    ...(options.env || {}),
    HTTP_ADDR: "127.0.0.1:0",
    DATABASE_URL: db.href,
    REDIS_URL: "unix://" + socket,
    ASSET_STORAGE_DIR: join(dir, "assets"),
    GRADER_BINARY_PATH: resolve("target/release/lms-grader"),
    COOKIE_SECURE: "false",
  };
  let child, worker, redis, schemaCreated = false;
  async function close() {
    try {
      await Promise.all([stop(worker), stop(child)]);
      await stop(redis);
      if (schemaCreated)
        await exec("psql", [source, "-Xq", "-v", "ON_ERROR_STOP=1", "-c", `DROP SCHEMA ${schema} CASCADE`]);
    } finally {
      await rm(dir, { recursive: true, force: true });
    }
  }
  try {
    await exec("psql", [source, "-Xq", "-v", "ON_ERROR_STOP=1", "-c", `CREATE SCHEMA ${schema}`]);
    schemaCreated = true;
    redis = spawn(process.env.TEST_REDIS_SERVER || "redis-server",
      ["--port", "0", "--unixsocket", socket, "--save", "", "--appendonly", "no", "--dir", dir],
      { stdio: ["ignore", "ignore", "inherit"] });
    redis.on("error", (error) => console.error(error.message));
    for (let attempt = 0; ; attempt++) {
      try {
        await exec("redis-cli", ["-s", socket, "ping"]);
        break;
      } catch (error) {
        if (attempt === 50) throw error;
        await delay(50);
      }
    }
    await exec("cargo", ["build", "--release", "-p", "lms-grader"]);
    const targets = ["./cmd/api", "./cmd/seed", ...(options.worker ? ["./cmd/worker"] : [])];
    await exec("go", ["build", "-o", dir + "/", ...targets], { cwd: "backend", env });
    await exec(join(dir, "seed"), [], { env });
    return {
      async start() {
        const apiStart = startAndWait(
          join(dir, "api"),
          env,
          "Go API",
          /"msg":"api listening","addr":"([^"]+)"/,
          (match) => "http://" + match[1],
        );
        child = apiStart.proc;
        if (options.worker) {
          const workerStart = startAndWait(
            join(dir, "worker"),
            env,
            "Go worker",
            /"msg":"worker running"/,
            () => undefined,
          );
          worker = workerStart.proc;
          const [baseUrl] = await Promise.all([apiStart.ready, workerStart.ready]);
          return baseUrl;
        }
        return apiStart.ready;
      },
      async stop() {
        await Promise.all([stop(worker), stop(child)]);
      },
      close,
    };
  } catch (cause) {
    await close();
    throw cause;
  }
}
