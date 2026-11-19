import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, readFile, readdir, rm, stat } from "node:fs/promises";
import { createServer } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const cli = fileURLToPath(
  new URL("../packages/cli/dist/cli.js", import.meta.url),
);
const email = "francetest@testtest.net";
const password = " mock password ";
const user = {
  id: "f86a0e00-6216-4595-a264-018b6d59e036",
  email,
  roles: [{ enName: "student" }],
};
const publicUser = { id: user.id, email, roles: ["student"] };

async function mockApi(t, options = {}) {
  const { expiresIn = 3600, refreshStatus = [] } = options;
  const requests = [];
  let rotations = 0;
  let active = 0;
  const server = createServer(async (request, response) => {
    let raw = "";
    for await (const chunk of request) raw += chunk;
    const path = request.url;
    requests.push(path);
    response.setHeader("Content-Type", "application/json");
    const reply = (status, data) => {
      response.statusCode = status;
      response.end(JSON.stringify(data));
    };
    const session = (version, ttl) => ({
      user,
      expiresIn: ttl,
      accessToken: `mock-access-${version}`,
      refreshToken: `mock-refresh-${version}`,
    });
    if (path === "/api/auth/login") {
      const credentials = JSON.parse(raw);
      if (credentials.email !== email || credentials.password !== password) {
        return reply(401, { message: "rejected" });
      }
      return reply(200, session(0, expiresIn));
    }
    if (path === "/api/auth/refresh") {
      const status = refreshStatus[rotations++] ?? 200;
      if (status !== 200)
        return reply(status, { message: "temporary or rejected" });
      if (JSON.parse(raw).refreshToken !== `mock-refresh-${active}`) {
        return reply(401, { message: "unexpected token" });
      }
      active++;
      return reply(200, session(active, 3600));
    }
    if (
      path === "/api/users/me" &&
      request.headers.authorization === `Bearer mock-access-${active}`
    ) {
      return reply(200, user);
    }
    reply(401, { message: "rejected" });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => new Promise((resolve) => server.close(resolve)));
  return { origin: `http://127.0.0.1:${server.address().port}`, requests };
}

async function setup(t, options) {
  const folder = await mkdtemp(join(tmpdir(), "sa2-cli-test-"));
  t.after(() => rm(folder, { recursive: true, force: true }));
  const { origin, requests } = await mockApi(t, options);
  const directory =
    process.platform === "darwin"
      ? join(folder, "Library", "Application Support", "sa2-cli")
      : join(folder, "sa2-cli");

  async function run(args, stdin = "") {
    const child = spawn(process.execPath, [cli, ...args], {
      env: {
        ...process.env,
        HOME: folder,
        XDG_STATE_HOME: folder,
        LOCALAPPDATA: folder,
        SA2_API_URL: origin,
      },
      stdio: ["pipe", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8").on("data", (chunk) => {
      stdout += chunk;
    });
    child.stderr.setEncoding("utf8").on("data", (chunk) => {
      stderr += chunk;
    });
    child.stdin.end(stdin);
    const [code] = await once(child, "close");
    assert.doesNotMatch(
      stdout + stderr,
      /mock password|mock-access|mock-refresh/,
    );
    assert.equal(stderr, "");
    return { code, result: JSON.parse(stdout) };
  }

  const login = () =>
    run(["login", "--email", email, "--password-stdin"], password + "\r\n");
  return { run, login, requests, directory };
}

test("login persists a protected session and logout is idempotent", async (t) => {
  const { run, login, requests, directory } = await setup(t);
  assert.deepEqual(await login(), { code: 0, result: { data: publicUser } });
  const [name] = await readdir(directory);
  const path = join(directory, name);
  if (process.platform === "win32") {
    assert.doesNotMatch(
      (await readFile(path)).toString(),
      /mock-(access|refresh)/,
    );
  } else {
    assert.equal((await stat(directory)).mode & 0o777, 0o700);
    assert.equal((await stat(path)).mode & 0o777, 0o600);
  }
  assert.deepEqual(await run(["whoami"]), {
    code: 0,
    result: { data: publicUser },
  });
  const other = await mockApi(t);
  assert.equal(
    (await run(["whoami", "--api-url", other.origin])).result.error.code,
    "LOGIN_REQUIRED",
  );
  assert.deepEqual(other.requests, []);
  for (let attempt = 0; attempt < 2; attempt++) {
    assert.deepEqual(await run(["logout"]), {
      code: 0,
      result: { data: { loggedOut: true } },
    });
  }
  assert.equal((await run(["whoami"])).result.error.code, "LOGIN_REQUIRED");
  assert.deepEqual(requests, ["/api/auth/login", "/api/users/me"]);
});

test("concurrent commands share one token rotation", async (t) => {
  const { run, login, requests } = await setup(t, { expiresIn: 1 });
  assert.equal((await login()).code, 0);
  for (const result of await Promise.all([run(["whoami"]), run(["whoami"])])) {
    assert.deepEqual(result, { code: 0, result: { data: publicUser } });
  }
  assert.equal(
    requests.filter((path) => path === "/api/auth/refresh").length,
    1,
  );
});

test("refresh failure retains the session until the API rejects it", async (t) => {
  const { run, login, requests } = await setup(t, {
    expiresIn: 1,
    refreshStatus: [503, 401],
  });
  assert.equal((await login()).code, 0);
  assert.equal((await run(["whoami"])).result.error.code, "API_UNAVAILABLE");
  assert.equal((await run(["whoami"])).result.error.code, "LOGIN_REQUIRED");
  assert.equal((await run(["whoami"])).result.error.code, "LOGIN_REQUIRED");
  assert.equal(
    requests.filter((path) => path === "/api/auth/refresh").length,
    2,
  );
});

test("validation rejects insecure URLs, bad credentials, and missing arguments", async (t) => {
  const { run, requests } = await setup(t);
  assert.equal(
    (await run(["whoami", "--api-url", "http://example.com"])).code,
    2,
  );
  assert.equal((await run(["login"])).result.error.code, "INVALID_ARGUMENT");
  const rejected = await run(
    ["login", "--email", email, "--password-stdin"],
    "wrong\n",
  );
  assert.equal(rejected.code, 3);
  assert.equal(rejected.result.error.code, "AUTH_REJECTED");
  assert.deepEqual(requests, ["/api/auth/login"]);
});
