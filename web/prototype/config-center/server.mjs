import { createReadStream } from "node:fs";
import { extname, join, normalize } from "node:path";
import { createServer } from "node:http";

const root = process.cwd();
const port = 4173;
const mime = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".svg": "image/svg+xml",
};

createServer((request, response) => {
  const pathname = decodeURIComponent(new URL(request.url, `http://${request.headers.host}`).pathname);
  const requested = pathname.endsWith("/") ? `${pathname}index.html` : pathname;
  const file = normalize(join(root, requested));

  if (!file.startsWith(root)) {
    response.writeHead(403).end("Forbidden");
    return;
  }

  const stream = createReadStream(file);
  stream.on("open", () => {
    response.writeHead(200, { "content-type": mime[extname(file)] ?? "application/octet-stream" });
    stream.pipe(response);
  });
  stream.on("error", () => response.writeHead(404).end("Not found"));
}).listen(port, "127.0.0.1", () => {
  console.log(`Prototype: http://127.0.0.1:${port}/prototype/config-center/?variant=structured`);
});
