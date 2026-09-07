#!/usr/bin/env node

const { spawn } = require("node:child_process");

const [rawSeconds, command, ...args] = process.argv.slice(2);
const seconds = Number(rawSeconds);

if (!Number.isInteger(seconds) || seconds < 1 || !command) {
  console.error("usage: run-with-timeout.cjs <seconds> <command> [args ...]");
  process.exit(2);
}

const child = spawn(command, args, {
  detached: process.platform !== "win32",
  stdio: "inherit",
});

let forcedExitCode = null;
let killTimer = null;
const rawGrace = Number(process.env.RCC_TIMEOUT_KILL_GRACE_MS || 5000);
const grace = Number.isInteger(rawGrace) && rawGrace >= 0 ? rawGrace : 5000;

function stop(signal) {
  try {
    if (process.platform === "win32") child.kill(signal);
    else process.kill(-child.pid, signal);
  } catch (error) {
    if (error.code !== "ESRCH") console.error(`failed to send ${signal}: ${error.message}`);
  }
}

function terminate(exitCode) {
  forcedExitCode = exitCode;
  stop("SIGTERM");
  if (!killTimer) killTimer = setTimeout(() => stop("SIGKILL"), grace);
}

const timer = setTimeout(() => {
  console.error(`command exceeded ${seconds}s timeout: ${command}`);
  terminate(124);
}, seconds * 1000);

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    terminate(128 + (signal === "SIGINT" ? 2 : 15));
  });
}

child.on("error", (error) => {
  clearTimeout(timer);
  if (killTimer) clearTimeout(killTimer);
  console.error(`failed to start ${command}: ${error.message}`);
  process.exitCode = forcedExitCode ?? 127;
});

child.on("exit", (code, signal) => {
  clearTimeout(timer);
  if (forcedExitCode !== null) process.exitCode = forcedExitCode;
  else if (code !== null) process.exitCode = code;
  else process.exitCode = 128 + (signal === "SIGKILL" ? 9 : 15);
});
