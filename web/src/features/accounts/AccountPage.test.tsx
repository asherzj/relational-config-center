import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { AccountPage } from "./AccountPage";

const identity = { account: { id: "ab09850e-ef9a-4317-a000-d67465416b5b", username: "alice", display_name: "小爱", email: "alice@example.com", email_verified: false, status: "enabled" }, csrf_token: "session-csrf", expires_at: "2026-09-07T08:00:00Z", idle_expires_at: "2026-09-07T00:30:00Z" };
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status, headers: { "Content-Type": "application/json" } });
const failure = (code: string, status: number) => json({ error: { code, message: "safe failure", request_id: "request-1" } }, status);
function TestAccountRoutes() { return <Routes><Route path="/register" element={<AccountPage key="register" mode="register" />} /><Route path="/login" element={<AccountPage key="login" mode="login" />} /><Route path="*" element={<AccountPage key="account" mode="account" />} /></Routes>; }
function renderAccount(path = "/register") { return render(<MemoryRouter initialEntries={[path]}><TestAccountRoutes /></MemoryRouter>); }

beforeEach(() => {
  const request = (_name: string, callback: () => Promise<unknown>) => callback();
  vi.stubGlobal("navigator", Object.assign(Object.create(navigator), { locks: { request } }));
});
afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); localStorage.clear(); });

it("registers with a prepared CSRF credential and presents the real current identity", async () => {
  let preparations = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/session")) return failure("session_invalid", 401);
    if (path.endsWith("/csrf")) { preparations++; return json({ csrf_token: "preauth-csrf" }); }
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
  expect(preparations).toBe(1);
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

it("updates its own profile and keeps the session on a wrong current password", async () => {
  let current = identity;
  const writes: string[] = [];
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(current);
    if (path.endsWith("/profile")) {
      writes.push(path);
      current = { ...current, account: { ...current.account, display_name: JSON.parse(String(init?.body)).display_name } };
      return json(current);
    }
    if (path.endsWith("/email")) {
      writes.push(path);
      const body = JSON.parse(String(init?.body));
      expect(body).toEqual({ email: "new@example.com", current_password: "wrong password long enough" });
      return failure("current_password_invalid", 400);
    }
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  const user = userEvent.setup();
  expect(await screen.findByText("小爱")).toBeVisible();
  await user.clear(screen.getByLabelText("显示名称"));
  await user.type(screen.getByLabelText("显示名称"), "新名称");
  await user.click(screen.getByRole("button", { name: "保存显示名称" }));
  expect(await screen.findByText("新名称")).toBeVisible();
  await user.clear(screen.getByLabelText("新邮箱"));
  await user.type(screen.getByLabelText("新邮箱"), "new@example.com");
  await user.type(screen.getByLabelText("当前密码", { selector: "#email-current-password" }), "wrong password long enough");
  await user.click(screen.getByRole("button", { name: "修改邮箱" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("当前密码不正确");
  expect(screen.getByText("新名称")).toBeVisible();
  expect(screen.getByText("alice@example.com")).toBeVisible();
  expect(writes).toEqual(["/api/v1/auth/profile", "/api/v1/auth/email"]);
});

it("changes the password and returns to login after every session is revoked", async () => {
  let changed = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/password")) {
      expect(init?.headers).toMatchObject({ "X-CSRF-Token": "session-csrf" });
      expect(JSON.parse(String(init?.body))).toEqual({ current_password: "current password long enough", new_password: "replacement password long enough" });
      changed = true;
      return new Response(null, { status: 204 });
    }
    if (path.endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  const user = userEvent.setup();
  await screen.findByText("小爱");
  await user.type(screen.getByLabelText("当前密码", { selector: "#password-current-password" }), "current password long enough");
  await user.type(screen.getByLabelText("新密码"), "replacement password long enough");
  await user.click(screen.getByRole("button", { name: "修改密码并退出全部设备" }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  expect(changed).toBe(true);
});

it("can exit every device from the current account", async () => {
  let revoked = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/logout-all")) {
      expect(init?.headers).toMatchObject({ "X-CSRF-Token": "session-csrf" });
      revoked = true;
      return new Response(null, { status: 204 });
    }
    if (path.endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  await screen.findByText("小爱");
  await userEvent.setup().click(screen.getByRole("button", { name: "退出全部设备" }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  expect(revoked).toBe(true);
});

it("reports visible foreground interaction at most once per 60 seconds without a heartbeat", async () => {
  let now = Date.parse("2026-09-07T00:00:00Z");
  vi.spyOn(Date, "now").mockImplementation(() => now);
  let activities = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/activity")) { activities++; return json(identity); }
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  await screen.findByText("小爱");
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(activities).toBe(0);
  fireEvent.pointerDown(document.body);
  await waitFor(() => expect(activities).toBe(1));
  fireEvent.keyDown(document.body, { key: "a" });
  expect(activities).toBe(1);
  now += 60_000;
  fireEvent.keyDown(document.body, { key: "b" });
  await waitFor(() => expect(activities).toBe(2));
});

it("serializes simultaneous activity reports from browser tabs", async () => {
  let releaseActivity!: (response: Response) => void;
  const pendingActivity = new Promise<Response>((resolve) => { releaseActivity = resolve; });
  let activities = 0;
  let lockTail: Promise<unknown> = Promise.resolve();
  const requestLock = vi.fn((_name: string, callback: () => Promise<unknown>) => {
    const result = lockTail.then(callback);
    lockTail = result.then(() => undefined, () => undefined);
    return result;
  });
  vi.stubGlobal("navigator", Object.assign(Object.create(navigator), { locks: { request: requestLock } }));
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/activity")) { activities++; return pendingActivity; }
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  renderAccount("/account");
  await screen.findAllByText("小爱");
  fireEvent.pointerDown(document.body);
  fireEvent.pointerDown(document.body);
  await waitFor(() => expect(requestLock).toHaveBeenCalledTimes(2));
  await waitFor(() => expect(activities).toBe(1));
  releaseActivity(json(identity));
  await lockTail;
  expect(activities).toBe(1);
});

it("clears the visible identity when another tab logs out the shared browser session", async () => {
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  expect(await screen.findByText("小爱")).toBeVisible();
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "ended", at: Date.now() }) }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  expect(screen.queryByText("小爱")).not.toBeInTheDocument();
});

it("does not restore a session from a late activity response after logout", async () => {
  let finishActivity!: (response: Response) => void;
  const pendingActivity = new Promise<Response>((resolve) => { finishActivity = resolve; });
  let activityStarted = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/activity")) { activityStarted = true; return pendingActivity; }
    if (path.endsWith("/logout")) return new Response(null, { status: 204 });
    if (path.endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  await screen.findByText("小爱");
  fireEvent.pointerDown(document.body);
  await waitFor(() => expect(activityStarted).toBe(true));
  await userEvent.setup().click(screen.getByRole("button", { name: "退出当前账号" }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  finishActivity(json(identity));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(screen.getByRole("heading", { name: "登录本地账号" })).toBeVisible();
});

it("does not overwrite an unfinished profile edit with an activity response", async () => {
  let finishActivity!: (response: Response) => void;
  const pendingActivity = new Promise<Response>((resolve) => { finishActivity = resolve; });
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/activity")) return pendingActivity;
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  const field = await screen.findByLabelText("显示名称");
  const user = userEvent.setup();
  await user.clear(field);
  await user.type(field, "正在编辑的新名称");
  finishActivity(json(identity));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(field).toHaveValue("正在编辑的新名称");
});

it("clears sensitive drafts before showing an account selected in another tab", async () => {
  const otherIdentity = { ...identity, account: { ...identity.account, id: "9e5e2b50-6aaa-4eaa-83fb-f56d84240214", username: "bob", display_name: "小博", email: "bob@example.com" }, csrf_token: "bob-csrf" };
  let reads = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(reads++ === 0 ? identity : otherIdentity);
    throw new Error(`unexpected ${path}`);
  }));
  localStorage.setItem("rcc:last-activity-report", String(Date.now()));
  renderAccount("/account");
  const user = userEvent.setup();
  await screen.findByText("小爱");
  await user.clear(screen.getByLabelText("新邮箱"));
  await user.type(screen.getByLabelText("新邮箱"), "private-draft@example.com");
  await user.type(screen.getByLabelText("当前密码", { selector: "#email-current-password" }), "email password draft");
  await user.type(screen.getByLabelText("当前密码", { selector: "#password-current-password" }), "password change draft");
  await user.type(screen.getByLabelText("新密码"), "new password draft");
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "changed", at: Date.now() }) }));
  expect(await screen.findByText("小博")).toBeVisible();
  expect(screen.getByLabelText("新邮箱")).toHaveValue("bob@example.com");
  expect(screen.getByLabelText("当前密码", { selector: "#email-current-password" })).toHaveValue("");
  expect(screen.getByLabelText("当前密码", { selector: "#password-current-password" })).toHaveValue("");
  expect(screen.getByLabelText("新密码")).toHaveValue("");
});

it("ignores a profile response that arrives after another tab ends the session", async () => {
  let finishProfile!: (response: Response) => void;
  const pendingProfile = new Promise<Response>((resolve) => { finishProfile = resolve; });
  let profileStarted = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(identity);
    if (path.endsWith("/profile")) { profileStarted = true; return pendingProfile; }
    if (path.endsWith("/csrf")) return json({ csrf_token: "new-preauth" });
    throw new Error(`unexpected ${path}`);
  }));
  localStorage.setItem("rcc:last-activity-report", String(Date.now()));
  renderAccount("/account");
  const user = userEvent.setup();
  await screen.findByText("小爱");
  await user.clear(screen.getByLabelText("显示名称"));
  await user.type(screen.getByLabelText("显示名称"), "迟到名称");
  await user.click(screen.getByRole("button", { name: "保存显示名称" }));
  await waitFor(() => expect(profileStarted).toBe(true));
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "ended", at: Date.now() }) }));
  expect(await screen.findByRole("heading", { name: "登录本地账号" })).toBeVisible();
  finishProfile(json({ ...identity, account: { ...identity.account, display_name: "迟到名称" } }));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(screen.getByRole("heading", { name: "登录本地账号" })).toBeVisible();
});

it("ignores an email response that arrives after another tab selects a new account", async () => {
  const otherIdentity = { ...identity, account: { ...identity.account, id: "9e5e2b50-6aaa-4eaa-83fb-f56d84240214", username: "bob", display_name: "小博", email: "bob@example.com" }, csrf_token: "bob-csrf" };
  let finishEmail!: (response: Response) => void;
  const pendingEmail = new Promise<Response>((resolve) => { finishEmail = resolve; });
  let reads = 0;
  let emailStarted = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(reads++ === 0 ? identity : otherIdentity);
    if (path.endsWith("/email")) { emailStarted = true; return pendingEmail; }
    throw new Error(`unexpected ${path}`);
  }));
  localStorage.setItem("rcc:last-activity-report", String(Date.now()));
  renderAccount("/account");
  const user = userEvent.setup();
  await screen.findByText("小爱");
  await user.clear(screen.getByLabelText("新邮箱"));
  await user.type(screen.getByLabelText("新邮箱"), "late@example.com");
  await user.type(screen.getByLabelText("当前密码", { selector: "#email-current-password" }), "current password long enough");
  await user.click(screen.getByRole("button", { name: "修改邮箱" }));
  await waitFor(() => expect(emailStarted).toBe(true));
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "changed", at: Date.now() }) }));
  expect(await screen.findByText("小博")).toBeVisible();
  finishEmail(json({ ...identity, account: { ...identity.account, email: "late@example.com" } }));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(screen.getByText("小博")).toBeVisible();
  expect(screen.getByText("bob@example.com")).toBeVisible();
  expect(screen.queryByText("late@example.com")).not.toBeInTheDocument();
});

it("ignores an initial session inspection that finishes after cross-tab logout", async () => {
  let finishSession!: (response: Response) => void;
  const pendingSession = new Promise<Response>((resolve) => { finishSession = resolve; });
  let sessionStarted = false;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) { sessionStarted = true; return pendingSession; }
    throw new Error(`unexpected ${input}`);
  }));
  renderAccount("/account");
  await waitFor(() => expect(sessionStarted).toBe(true));
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "ended", at: Date.now() }) }));
  finishSession(json(identity));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(screen.getByRole("heading", { name: "登录本地账号" })).toBeVisible();
  expect(screen.queryByText("小爱")).not.toBeInTheDocument();
});

it("ignores a stale activity failure after another tab selects a new account", async () => {
  const otherIdentity = { ...identity, account: { ...identity.account, id: "9e5e2b50-6aaa-4eaa-83fb-f56d84240214", username: "bob", display_name: "小博", email: "bob@example.com" }, csrf_token: "bob-csrf" };
  let finishActivity!: (response: Response) => void;
  const pendingActivity = new Promise<Response>((resolve) => { finishActivity = resolve; });
  let reads = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    const path = String(input);
    if (path.endsWith("/session")) return json(reads++ === 0 ? identity : otherIdentity);
    if (path.endsWith("/activity")) return pendingActivity;
    throw new Error(`unexpected ${path}`);
  }));
  renderAccount("/account");
  await screen.findByText("小爱");
  fireEvent.pointerDown(document.body);
  window.dispatchEvent(new StorageEvent("storage", { key: "rcc:session-event", newValue: JSON.stringify({ type: "changed", at: Date.now() }) }));
  expect(await screen.findByText("小博")).toBeVisible();
  finishActivity(failure("session_invalid", 401));
  await new Promise((resolve) => setTimeout(resolve, 5));
  expect(screen.getByText("小博")).toBeVisible();
  expect(screen.queryByRole("heading", { name: "登录本地账号" })).not.toBeInTheDocument();
});

it("does not mint a pre-login credential during account-route changes", async () => {
  let preparations = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return failure("session_invalid", 401);
    preparations++;
    return json({ csrf_token: "unused-preauth" });
  }));
  renderAccount("/register");
  await screen.findByRole("button", { name: "注册并登录" });
  await userEvent.setup().click(screen.getByRole("link", { name: "已有账号，去登录" }));
  await screen.findByRole("heading", { name: "登录本地账号" });
  expect(screen.getByRole("button", { name: "登录" })).toBeEnabled();
  expect(preparations).toBe(0);
});


it("does not mint duplicate pre-login credentials under StrictMode", async () => {
  let preparations = 0;
  vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
    if (String(input).endsWith("/session")) return failure("session_invalid", 401);
    preparations++;
    return json({ csrf_token: "strict-mode-preauth" });
  }));
  render(<StrictMode><MemoryRouter initialEntries={["/register"]}><AccountPage mode="register" /></MemoryRouter></StrictMode>);
  await waitFor(() => expect(screen.getByRole("button", { name: "注册并登录" })).toBeEnabled());
  expect(preparations).toBe(0);
});
