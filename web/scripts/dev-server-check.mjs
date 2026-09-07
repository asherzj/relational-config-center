import assert from "node:assert/strict";
import { once } from "node:events";
import { createServer as createHTTPServer } from "node:http";
import { test } from "node:test";
import { createServer, loadConfigFromFile } from "vite";

async function listen(server) {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  return server.address().port;
}

async function preview(t, env) {
  for (const [name, value] of Object.entries(env)) {
    const previous = process.env[name];
    process.env[name] = value;
    t.after(() => {
      if (previous === undefined) delete process.env[name];
      else process.env[name] = previous;
    });
  }
  const { config } = await loadConfigFromFile({ command: "serve", mode: "development" });
  const server = await createServer({
    ...config,
    configFile: false,
    logLevel: "silent",
    server: { ...config.server, watch: null, hmr: false },
    optimizeDeps: { noDiscovery: true, include: [] },
  });
  t.after(() => server.close());
  return server;
}

test("uses the configured browser port and preserves Origin through the Admin proxy", async (t) => {
  const upstream = createHTTPServer((request, response) => {
    response.setHeader("Content-Type", "application/json");
    response.end(JSON.stringify({ origin: request.headers.origin }));
  });
  const upstreamPort = await listen(upstream);
  t.after(() => new Promise((resolve) => upstream.close(resolve)));
  const reservation = createHTTPServer();
  const port = await listen(reservation);
  await new Promise((resolve) => reservation.close(resolve));
  const server = await preview(t, {
    RCC_WEB_PORT: String(port),
    RCC_ADMIN_URL: `http://127.0.0.1:${upstreamPort}`,
  });
  await server.listen();
  assert.equal(server.httpServer.address().port, port);
  const origin = `http://127.0.0.1:${port}`;
  for (const requestOrigin of [origin, "https://untrusted.example"]) {
    const response = await fetch(`${origin}/api/v1/auth/login`, {
      method: "POST",
      headers: { Origin: requestOrigin },
    });
    assert.deepEqual(await response.json(), { origin: requestOrigin });
  }
});

test("fails when the configured port is occupied instead of changing the login origin", async (t) => {
  const occupied = createHTTPServer();
  const port = await listen(occupied);
  t.after(() => new Promise((resolve) => occupied.close(resolve)));
  const server = await preview(t, { RCC_WEB_PORT: String(port) });
  await assert.rejects(server.listen(), new RegExp(`Port ${port} is already in use`));
});
