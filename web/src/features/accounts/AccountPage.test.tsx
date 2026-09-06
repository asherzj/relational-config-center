import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, expect, it, vi } from "vitest";
import { AppRoutes } from "../../app";

const identity = { account: { id: "ab09850e-ef9a-4317-a000-d67465416b5b", username: "alice", display_name: "小爱", email: "alice@example.com", email_verified: false, status: "enabled" }, csrf_token: "session-csrf", expires_at: "2026-09-07T08:00:00Z", idle_expires_at: "2026-09-07T00:30:00Z" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const failure = (code: string, status: number) => json({ error: { code, message: "safe failure", request_id: "request-1" } }, status);
function renderAccount(path = "/register") { return render(<MemoryRouter initialEntries={[path]}><AppRoutes /></MemoryRouter>); }
afterEach(() => vi.unstubAllGlobals());

it("registers with a prepared CSRF credential and presents the real current identity", async () => {
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/session")) return failure("session_invalid", 401);
    if (path.endsWith("/csrf")) return json({ csrf_token: "preauth-csrf" });
    if (path.endsWith("/register")) {
      expect(init?.headers).toMatchObject({ "X-CSRF-Token": "preauth-csrf" });
      expect(JSON.parse(String(init?.body))).toMatchObject({ username: "alice", password: " exact password with spaces " });
      return json(identity, 201);
    }
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount();
  expect(await screen.findByRole("heading", { name: "注册本地账号" })).toBeVisible();
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("用户名"), "alice");
  await user.type(screen.getByLabelText("邮箱"), "alice@example.com");
  await user.type(screen.getByLabelText("密码"), " exact password with spaces ");
  await user.click(screen.getByRole("button", { name: "注册并登录" }));
  expect(await screen.findByText("小爱")).toBeVisible();
  expect(screen.getByText("alice")).toBeVisible();
  expect(screen.getByText(/邮箱未验证/)).toBeVisible();
});

it.each([
  ["invalid_credentials", 401, "用户名或密码不正确"],
  ["account_conflict", 409, "用户名或邮箱已被占用"],
  ["auth_rate_limited", 429, "等待 42 秒"],
  ["csrf_invalid", 403, "请求来源不正确"],
])("explains %s and does not automatically resubmit", async (code, status, message) => {
  let writes = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return failure("session_invalid", 401);
    if (String(input).endsWith("/csrf")) return json({ csrf_token: "preauth-csrf" });
    writes++;
    const response = failure(String(code), Number(status));
    if (status === 429) response.headers.set("Retry-After", "42");
    return response;
  }));
  renderAccount("/login");
  await screen.findByRole("button", { name: "登录" });
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("用户名"), "alice");
  await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
  await user.click(screen.getByRole("button", { name: "登录" }));
  expect(await screen.findByRole("alert")).toHaveTextContent(String(message));
  expect(writes).toBe(1);
});

it("offers session rechecking after an uncertain registration without replaying it", async () => {
  let writes = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return writes ? json(identity) : failure("session_invalid", 401);
    if (String(input).endsWith("/csrf")) return json({ csrf_token: "preauth-csrf" });
    writes++;
    throw new TypeError("response lost");
  }));
  renderAccount();
  const user = userEvent.setup();
  await screen.findByRole("button", { name: "注册并登录" });
  await user.type(screen.getByLabelText("用户名"), "alice");
  await user.type(screen.getByLabelText("邮箱"), "alice@example.com");
  await user.type(screen.getByLabelText("密码"), "correct horse battery staple");
  await user.click(screen.getByRole("button", { name: "注册并登录" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("提交结果尚未确认");
  await user.click(screen.getByRole("button", { name: "重新检查登录状态" }));
  expect(await screen.findByText("小爱")).toBeVisible();
  expect(writes).toBe(1);
});

it("shows restored identity and removes it after logout", async () => {
  let logout = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input).endsWith("/session")) return json(identity);
    if (String(input).endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    expect(String(input)).toBe("/api/v1/auth/logout");
    expect(init?.headers).toMatchObject({ "X-CSRF-Token": "session-csrf" });
    logout = true;
    return new Response(null, { status: 204 });
  }));
  renderAccount("/account");
  expect(await screen.findByText("小爱")).toBeVisible();
  await userEvent.setup().click(screen.getByRole("button", { name: "退出当前账号" }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  expect(logout).toBe(true);
  expect(screen.queryByText("小爱")).not.toBeInTheDocument();
});

it("shares pending preparation across account-route changes", async () => {
  let preparations = 0;
  let finishPreparation!: (response: Response) => void;
  const pendingPreparation = new Promise<Response>((resolve) => { finishPreparation = resolve; });
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return failure("session_invalid", 401);
    preparations++;
    return pendingPreparation;
  }));
  renderAccount("/register");
  await waitFor(() => expect(preparations).toBe(1));
  await userEvent.setup().click(screen.getByRole("link", { name: "已有账号，去登录" }));
  await screen.findByRole("heading", { name: "登录本地账号" });
  finishPreparation(json({ csrf_token: "shared-preauth" }));
  await waitFor(() => expect(screen.getByRole("button", { name: "登录" })).toBeEnabled());
  expect(preparations).toBe(1);
});


it("prepares one matching credential under the application's StrictMode", async () => {
  let preparations = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return failure("session_invalid", 401);
    preparations++;
    return json({ csrf_token: "strict-mode-preauth" });
  }));
  render(<StrictMode><MemoryRouter initialEntries={["/register"]}><AppRoutes /></MemoryRouter></StrictMode>);
  await waitFor(() => expect(screen.getByRole("button", { name: "注册并登录" })).toBeEnabled());
  expect(preparations).toBe(1);
});
