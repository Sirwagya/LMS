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

export async function isolatedGoServer() {
  const source = process.env.TEST_DATABASE_URL || defaultDB;
  const schema = "lms_http_" + randomBytes(10).toString("hex");
  const db = new URL(source);
  db.searchParams.set("search_path", `${schema},public`);
  const dir = await mkdtemp("/tmp/lms-http-");
  const socket = join(dir, "redis.sock");
  const env = {
    ...process.env,
    HTTP_ADDR: "127.0.0.1:0",
    DATABASE_URL: db.href,
    REDIS_URL: "unix://" + socket,
    ASSET_STORAGE_DIR: join(dir, "assets"),
    GRADER_BINARY_PATH: resolve("target/release/lms-grader"),
    COOKIE_SECURE: "false",
  };
  let child, redis, schemaCreated = false;
  async function close() {
    try {
      await stop(child);
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
    await exec("go", ["build", "-o", dir + "/", "./cmd/api", "./cmd/seed"], { cwd: "backend", env });
    await exec(join(dir, "seed"), [], { env });
    return {
      async start() {
        child = spawn(join(dir, "api"), [], { env, stdio: ["ignore", "ignore", "pipe"] });
        return new Promise((resolve, reject) => {
          let logs = "";
          const timer = setTimeout(() => reject(new Error("Go API startup timed out\n" + logs)), 10000);
          child.once("error", (error) => { clearTimeout(timer); reject(error); });
          child.once("exit", (code) => {
            clearTimeout(timer);
            reject(new Error(`Go API exited ${code}\n${logs}`));
          });
          child.stderr.on("data", (data) => {
            logs += data;
            const addr = logs.match(/"msg":"api listening","addr":"([^"]+)"/)?.[1];
            if (addr) {
              clearTimeout(timer);
              resolve("http://" + addr);
            }
          });
        });
      },
      stop: () => stop(child),
      close,
    };
  } catch (cause) {
    await close();
    throw cause;
  }
}
