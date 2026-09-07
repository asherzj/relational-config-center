const path = require("node:path");
const { pathToFileURL } = require("node:url");

const [webRoot, ...args] = process.argv.slice(2);
if (!webRoot) {
  console.error("usage: run-vite-preview.cjs <web-root> [vite preview args ...]");
  process.exit(2);
}

const viteEntry = path.join(webRoot, "node_modules/vite/bin/vite.js");
process.chdir(webRoot);
process.argv = [process.execPath, viteEntry, "preview", ...args];
import(pathToFileURL(viteEntry).href).catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
