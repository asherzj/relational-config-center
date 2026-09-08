// @vitest-environment node
import { createServer } from "node:http";
import { expect, it } from "vitest";
import { request } from "./client";

it("retains received HTTP status and Request ID after a real TCP response body interruption", async () => {
  let writes = 0;
  const server = createServer((req, res) => {
    if (req.method === "POST") writes++;
    res.writeHead(201, { "Content-Type": "application/json", "Content-Length": "100", "X-Request-ID": "req-tcp-body" });
    res.flushHeaders();
    res.write('{"id":');
    setTimeout(() => res.destroy(), 30);
  });
  await new Promise<void>(resolve => server.listen(0, "127.0.0.1", resolve));
  try {
    const address = server.address();
    if (!address || typeof address === "string") throw new Error("missing TCP address");
    await expect(request(`http://127.0.0.1:${address.port}/rows`, { method: "POST", body: "{}" })).rejects.toMatchObject({
      name: "ApiError", code: "network_error", status: 201, requestId: "req-tcp-body",
    });
    expect(writes).toBe(1);
  } finally {
    server.closeAllConnections();
    await new Promise<void>((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
  }
});
