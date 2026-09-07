const { spawn } = require("node:child_process");

const path = require("node:path");

const descendant = spawn(process.execPath, [
  path.join(__dirname, "timeout-resistant-descendant.cjs"),
], { stdio: ["ignore", "ignore", "ignore", "ipc"] });

descendant.once("message", () => console.log(descendant.pid));
process.on("SIGTERM", () => process.exit(0));
setInterval(() => {}, 1000);
