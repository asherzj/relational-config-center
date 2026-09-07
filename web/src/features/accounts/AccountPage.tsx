import { Database } from "lucide-react";
import { Input } from "../../components/shadcn/input";
import { Label } from "../../components/shadcn/label";
import { accountEntryLockName, withBrowserLock, publishSessionEvent, subscribeSessionEvents } from "./sessionCoordination";
import { useForegroundActivity } from "./useForegroundActivity";
import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { safeReturnDestination } from "./returnDestination";
import { accounts, type CurrentIdentity } from "../../api/accounts";
import { ApiError, isUncertainWriteError } from "../../api/client";
import { Button } from "../../components/ui/Button";

function formatSessionTime(value: string) {
  return `${new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" }).format(new Date(value))} UTC`;
}

type ErrorContext = "read" | "entry-write" | "account-write";

function errorMessage(error: unknown, context: ErrorContext): string {
  if (!(error instanceof ApiError)) return "服务暂时不可用，请重新检查登录状态。";
  if (error.code === "account_conflict") return "用户名或邮箱已被占用。若刚才注册的结果未确认，请用原用户名和密码登录。";
  if (error.code === "invalid_credentials") return "用户名或密码不正确，请检查后再试。";
  if (error.code === "current_password_invalid") return "当前密码不正确，资料和登录状态均未改变。";
  if (error.code === "invalid_account_fields") return "请检查用户名、邮箱、显示名称和密码是否符合下方规则。";
  if (error.status === 429) return `尝试过于频繁，请等待${error.retryAfter ? ` ${error.retryAfter} 秒` : "限速窗口结束"}后再提交。`;
  if (error.status === 403) return "登录凭据已失效或请求来源不正确。请重新检查登录状态后再提交。";
  if (context !== "read" && isUncertainWriteError(error)) {
    return context === "entry-write"
      ? "提交结果尚未确认。请重新检查登录状态；注册可能已经成功，也可以使用原用户名和密码登录。请勿重复提交。"
      : "提交结果尚未确认。请重新检查当前账号状态，不要自动重复资料修改或退出操作。";
  }
  return "账号服务暂时不可用，原登录凭据已保留，请稍后重新检查登录状态。";
}

export function AccountPage({ mode, onSignedIn, onSessionInvalid }: { mode: "login" | "register" | "account"; onSignedIn?: (identity: CurrentIdentity) => void; onSessionInvalid?: (code: string) => void }) {
  const [searchParams] = useSearchParams();
  const destination = safeReturnDestination(searchParams.get("returnTo"));
  const returnQuery = `?returnTo=${encodeURIComponent(destination)}`;
  const [identity, setIdentity] = useState<CurrentIdentity | null>(null);
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [emailCurrentPassword, setEmailCurrentPassword] = useState("");
  const [passwordCurrent, setPasswordCurrent] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [notice, setNotice] = useState("");
  const [writeUncertain, setWriteUncertain] = useState(false);
  const identityRef = useRef<CurrentIdentity | null>(null);
  const sessionGeneration = useRef(0);

  function refreshIdentity(current: CurrentIdentity) {
    identityRef.current = current;
    setIdentity(current);
  }
  function establishIdentity(current: CurrentIdentity) {
    sessionGeneration.current++;
    refreshIdentity(current);
    setBusy(false);
    setDisplayName(current.account.display_name);
    setEmail(current.account.email);
    setPassword("");
    setEmailCurrentPassword("");
    setPasswordCurrent("");
    setNewPassword("");
    setWriteUncertain(false);
  }
  function clearAccountState() {
    sessionGeneration.current++;
    identityRef.current = null;
    setIdentity(null);
    setBusy(false);
    setUsername("");
    setEmail("");
    setDisplayName("");
    setPassword("");
    setEmailCurrentPassword("");
    setPasswordCurrent("");
    setNewPassword("");
    setNotice("");
    setWriteUncertain(false);
  }
  function stillCurrent(generation: number, sessionCSRF: string) {
    return sessionGeneration.current === generation && identityRef.current?.csrf_token === sessionCSRF;
  }

  async function inspect() {
    const generation = sessionGeneration.current;
    setBusy(true); setError("");
    try {
      const current = await accounts.current();
      if (sessionGeneration.current === generation) establishIdentity(current);
    } catch (cause) {
      if (sessionGeneration.current !== generation) return;
      if (cause instanceof ApiError && cause.status === 401) {
        onSessionInvalid?.(cause.code);
        clearAccountState();
      } else { setError(errorMessage(cause, "read")); }
    } finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  useEffect(() => {
    let active = true;
    const generation = sessionGeneration.current;
    void (async () => {
      try {
        const current = await accounts.current();
        if (active && sessionGeneration.current === generation) establishIdentity(current);
      } catch (cause) {
        if (!active || sessionGeneration.current !== generation) return;
        if (cause instanceof ApiError && cause.status === 401) onSessionInvalid?.(cause.code);
        else setError(errorMessage(cause, "read"));
      } finally { if (active && sessionGeneration.current === generation) setBusy(false); }
    })();
    return () => { active = false; };
  }, []);
  useEffect(() => {
    const synchronize = (type: "changed" | "ended") => {
      if (type === "changed") {
        clearAccountState();
        void inspect();
        return;
      }
      if (type !== "ended") return;
      clearAccountState(); setError("");
    };
    return subscribeSessionEvents(synchronize);
  }, []);
  useForegroundActivity(identity, {
    generation: () => sessionGeneration.current,
    isCurrent: stillCurrent,
    onCurrent: refreshIdentity,
    onInvalid: clearAccountState,
    onError: (cause) => setError(errorMessage(cause, "read")),
  });

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy) return;
    const generation = sessionGeneration.current;
    setBusy(true); setError("");
    try {
      const current = await withBrowserLock(accountEntryLockName, async () => {
        if (sessionGeneration.current !== generation) return null;
        const prepared = await accounts.prepare();
        if (sessionGeneration.current !== generation) return null;
        return mode === "register"
          ? accounts.register({ username, email, password, ...(displayName ? { display_name: displayName } : {}) }, prepared.csrf_token)
          : accounts.login({ username, password }, prepared.csrf_token);
      });
      if (!current || sessionGeneration.current !== generation) return;
      establishIdentity(current); publishSessionEvent("changed");
      onSignedIn?.(current);
    } catch (cause) { setWriteUncertain(mode === "register" && isUncertainWriteError(cause)); setError(errorMessage(cause, "entry-write")); }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  async function logout() {
    if (!identity || busy) return;
    const generation = sessionGeneration.current; const sessionCSRF = identity.csrf_token;
    setBusy(true); setError("");
    try {
      const completed = await withBrowserLock(accountEntryLockName, async () => {
        if (!stillCurrent(generation, sessionCSRF)) return false;
        await accounts.logout(sessionCSRF);
        return true;
      });
      if (!completed || !stillCurrent(generation, sessionCSRF)) return;
      publishSessionEvent("ended"); clearAccountState();
    }
    catch (cause) { if (stillCurrent(generation, sessionCSRF)) { setWriteUncertain(isUncertainWriteError(cause)); setError(errorMessage(cause, "account-write")); } }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  async function updateDisplayName(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!identity || busy) return;
    const generation = sessionGeneration.current; const sessionCSRF = identity.csrf_token;
    setBusy(true); setError(""); setNotice("");
    try { const current = await accounts.updateDisplayName(displayName, sessionCSRF); if (!stillCurrent(generation, sessionCSRF)) return; refreshIdentity(current); setDisplayName(current.account.display_name); setNotice("显示名称已更新。"); }
    catch (cause) { if (stillCurrent(generation, sessionCSRF)) { setWriteUncertain(isUncertainWriteError(cause)); setError(errorMessage(cause, "account-write")); } }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  async function updateEmail(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!identity || busy) return;
    const generation = sessionGeneration.current; const sessionCSRF = identity.csrf_token;
    setBusy(true); setError(""); setNotice("");
    try { const current = await accounts.updateEmail(email, emailCurrentPassword, sessionCSRF); if (!stillCurrent(generation, sessionCSRF)) return; refreshIdentity(current); setEmail(current.account.email); setEmailCurrentPassword(""); setNotice("邮箱已更新，仍处于未验证状态。"); }
    catch (cause) { if (stillCurrent(generation, sessionCSRF)) { setWriteUncertain(isUncertainWriteError(cause)); setError(errorMessage(cause, "account-write")); } }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  async function changePassword(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (!identity || busy) return;
    const generation = sessionGeneration.current; const sessionCSRF = identity.csrf_token;
    setBusy(true); setError(""); setNotice("");
    try {
      const completed = await withBrowserLock(accountEntryLockName, async () => {
        if (!stillCurrent(generation, sessionCSRF)) return false;
        await accounts.changePassword(passwordCurrent, newPassword, sessionCSRF);
        return true;
      });
      if (!completed || !stillCurrent(generation, sessionCSRF)) return;
      publishSessionEvent("ended");
      clearAccountState();
    } catch (cause) { if (stillCurrent(generation, sessionCSRF)) { setWriteUncertain(isUncertainWriteError(cause)); setError(errorMessage(cause, "account-write")); } }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }
  async function logoutAll() {
    if (!identity || busy) return;
    const generation = sessionGeneration.current; const sessionCSRF = identity.csrf_token;
    setBusy(true); setError(""); setNotice("");
    try {
      const completed = await withBrowserLock(accountEntryLockName, async () => {
        if (!stillCurrent(generation, sessionCSRF)) return false;
        await accounts.logoutAll(sessionCSRF);
        return true;
      });
      if (!completed || !stillCurrent(generation, sessionCSRF)) return;
      publishSessionEvent("ended");
      clearAccountState();
    } catch (cause) { if (stillCurrent(generation, sessionCSRF)) { setWriteUncertain(isUncertainWriteError(cause)); setError(errorMessage(cause, "account-write")); } }
    finally { if (sessionGeneration.current === generation) setBusy(false); }
  }

  return <main className={`account-page account-page-${identity ? "settings" : mode}`}>
    <section className="account-card" aria-busy={busy}>
      <Link className="account-brand" to="/account"><span className="account-brand-mark"><Database size={22} strokeWidth={1.6} /></span>关系型配置中心</Link>
      <h1>{identity ? "当前账号" : mode === "register" ? "注册本地账号" : "登录本地账号"}</h1>
      {error && <div className="account-error" role="alert">{error}<Button onClick={() => void inspect()} disabled={busy}>重新检查登录状态</Button></div>}
      {notice && <p className="account-notice" role="status">{notice}</p>}
      {identity ? <>
        <dl className="account-details"><dt>显示名称</dt><dd>{identity.account.display_name}</dd><dt>用户名</dt><dd>{identity.account.username}</dd><dt>邮箱</dt><dd>{identity.account.email}</dd><dt>闲置到期</dt><dd><time dateTime={identity.idle_expires_at}>{formatSessionTime(identity.idle_expires_at)}</time></dd><dt>最晚到期</dt><dd><time dateTime={identity.expires_at}>{formatSessionTime(identity.expires_at)}</time></dd></dl>
        <p className="account-help">邮箱未验证，不用于登录或找回密码。</p>
        <Link className="account-workspace-link" to={destination}>进入管理工作区</Link>
        <form className="account-action" onSubmit={updateDisplayName}>
          <h2>个人资料</h2>
          <Label>显示名称<Input name="display_name" autoComplete="nickname" value={displayName} onChange={(event) => setDisplayName(event.target.value)} required /></Label>
          <Button type="submit" disabled={busy || writeUncertain}>保存显示名称</Button>
        </form>
        <form className="account-action" onSubmit={updateEmail}>
          <h2>修改未验证邮箱</h2>
          <Label>新邮箱<Input name="email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></Label>
          <Label htmlFor="email-current-password">当前密码<Input id="email-current-password" type="password" autoComplete="current-password" value={emailCurrentPassword} onChange={(event) => setEmailCurrentPassword(event.target.value)} required /></Label>
          <Button type="submit" disabled={busy || writeUncertain}>修改邮箱</Button>
        </form>
        <form className="account-action" onSubmit={changePassword}>
          <h2>修改密码</h2>
          <Label htmlFor="password-current-password">当前密码<Input id="password-current-password" type="password" autoComplete="current-password" value={passwordCurrent} onChange={(event) => setPasswordCurrent(event.target.value)} required /></Label>
          <Label>新密码<Input type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} required /></Label>
          <p className="account-help">成功后所有设备都需要用新密码重新登录。</p>
          <Button type="submit" disabled={busy || writeUncertain}>修改密码并退出全部设备</Button>
        </form>
        <div className="account-session-actions">
        <Button onClick={() => void logout()} disabled={busy || writeUncertain}>退出当前账号</Button>
        <Button variant="danger" onClick={() => void logoutAll()} disabled={busy || writeUncertain}>退出全部设备</Button>
        </div>
      </> : <form onSubmit={submit}>
        <Label>用户名<Input name="username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /></Label>
        {mode === "register" && <><p className="account-help">3–32 个 ASCII 字符，以字母开头，可含数字、点、下划线和连字符；忽略首尾空白及大小写。</p>
          <Label>邮箱<Input name="email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></Label>
          <p className="account-help">必填且唯一。邮箱未验证，本轮不发送邮件，不用于登录或找回密码。</p>
          <Label>显示名称（可选）<Input name="display_name" autoComplete="nickname" value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></Label>
          <p className="account-help">省略时使用用户名；填写时为 1–64 个字符，不含控制字符。</p></>}
        <Label>密码<Input name="password" type="password" autoComplete={mode === "register" ? "new-password" : "current-password"} value={password} onChange={(event) => setPassword(event.target.value)} required /></Label>
        <p className="account-help">15–128 个字符；空格和大小写都会保留。</p>
        <Button type="submit" variant="primary" disabled={busy || writeUncertain}>{busy ? "正在处理…" : mode === "register" ? "注册并登录" : "登录"}</Button>
        <p className="account-link">{mode === "register" ? <Link to={`/login${returnQuery}`}>已有账号，去登录</Link> : <Link to={`/register${returnQuery}`}>注册新账号</Link>}</p>
      </form>}
    </section>
  </main>;
}
