const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { test } = require("node:test");

test("kills the process group when its leader exits on TERM", async () => {
  if (process.platform === "win32") return;

  const root = __dirname;
  const result = spawnSync(process.execPath, [
    path.join(root, "run-with-timeout.cjs"),
    "1",
    process.execPath,
    path.join(root, "fixtures/timeout-process-tree.cjs"),
  ], {
    encoding: "utf8",
    env: { ...process.env, RCC_TIMEOUT_KILL_GRACE_MS: "200" },
    timeout: 10000,
  });

  assert.equal(result.status, 124, result.stderr);
  const descendantPID = Number(result.stdout.trim());
  assert.ok(Number.isInteger(descendantPID) && descendantPID > 1, result.stdout);

  let alive = true;
  try {
    try {
      process.kill(descendantPID, 0);
      if (process.platform === "linux") {
        const state = fs.readFileSync(`/proc/${descendantPID}/stat`, "utf8").split(" ")[2];
        if (state === "Z") alive = false;
      }
    } catch (error) {
      if (error.code === "ESRCH") alive = false;
      else throw error;
    }
    assert.equal(alive, false, `descendant ${descendantPID} survived the timeout`);
  } finally {
    if (alive) {
      try { process.kill(descendantPID, "SIGKILL"); } catch (error) {
        if (error.code !== "ESRCH") throw error;
      }
    }
  }
});
