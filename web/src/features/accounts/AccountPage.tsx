import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { accounts, type CurrentIdentity } from "../../api/accounts";
import { ApiError } from "../../api/client";
import { Button } from "../../components/ui/Button";

function errorMessage(error: unknown, submitting: boolean): string {
  if (!(error instanceof ApiError)) return "服务暂时不可用，请重新检查登录状态。";
  if (error.code === "account_conflict") return "用户名或邮箱已被占用。若刚才注册的结果未确认，请用原用户名和密码登录。";
  if (error.code === "invalid_credentials") return "用户名或密码不正确，请检查后再试。";
  if (error.code === "invalid_account_fields") return "请检查用户名、邮箱、显示名称和密码是否符合下方规则。";
  if (error.status === 429) return `尝试过于频繁，请等待${error.retryAfter ? ` ${error.retryAfter} 秒` : "限速窗口结束"}后再提交。`;
  if (error.status === 403) return "登录凭据已失效或请求来源不正确。请重新检查登录状态后再提交。";
  if (submitting && (error.code === "network_error" || error.status === 504 || error.code === "contract_mismatch")) return "提交结果尚未确认。请重新检查登录状态；注册可能已经成功，也可以使用原用户名和密码登录。请勿重复提交。";
  return "账号服务暂时不可用，原登录凭据已保留，请稍后重新检查登录状态。";
}

export function AccountPage({ mode }: { mode: "login" | "register" | "account" }) {
  const [identity, setIdentity] = useState<CurrentIdentity | null>(null);
  const [csrf, setCSRF] = useState("");
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");

  async function inspect() {
    setBusy(true); setError("");
    try {
      const current = await accounts.current();
      setIdentity(current); setPassword("");
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        setIdentity(null);
        try { setCSRF((await accounts.prepare()).csrf_token); } catch (prepareError) { setError(errorMessage(prepareError, false)); }
      } else { setError(errorMessage(cause, false)); }
    } finally { setBusy(false); }
  }
  useEffect(() => {
    let active = true;
    void (async () => {
      try {
        const current = await accounts.current();
        if (active) setIdentity(current);
      } catch (cause) {
        if (!active) return;
        if (cause instanceof ApiError && cause.status === 401) {
          try { const prepared = await accounts.prepare(); if (active) setCSRF(prepared.csrf_token); }
          catch (prepareError) { if (active) setError(errorMessage(prepareError, false)); }
        } else if (active) setError(errorMessage(cause, false));
      } finally { if (active) setBusy(false); }
    })();
    return () => { active = false; };
  }, []);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); if (busy || !csrf) return;
    setBusy(true); setError("");
    try {
      const current = mode === "register"
        ? await accounts.register({ username, email, password, ...(displayName ? { display_name: displayName } : {}) }, csrf)
        : await accounts.login({ username, password }, csrf);
      setIdentity(current); setCSRF(""); setPassword("");
    } catch (cause) { setError(errorMessage(cause, true)); }
    finally { setBusy(false); }
  }
  async function logout() {
    if (!identity || busy) return;
    setBusy(true); setError("");
    try { await accounts.logout(identity.csrf_token); setIdentity(null); setUsername(""); setEmail(""); setDisplayName(""); setPassword(""); setCSRF((await accounts.prepare()).csrf_token); }
    catch (cause) { setError(errorMessage(cause, true)); }
    finally { setBusy(false); }
  }

  return <main className="account-page">
    <section className="account-card" aria-busy={busy}>
      <Link className="account-brand" to="/account">关系型配置中心</Link>
      <h1>{identity ? "当前账号" : mode === "register" ? "注册本地账号" : "登录本地账号"}</h1>
      {error && <div className="account-error" role="alert">{error}<Button onClick={() => void inspect()} disabled={busy}>重新检查登录状态</Button></div>}
      {identity ? <>
        <dl className="account-details"><dt>显示名称</dt><dd>{identity.account.display_name}</dd><dt>用户名</dt><dd>{identity.account.username}</dd><dt>邮箱</dt><dd>{identity.account.email}</dd></dl>
        <p className="account-help">邮箱未验证，不用于登录或找回密码。</p>
        <Button onClick={() => void logout()} disabled={busy}>退出当前账号</Button>
      </> : <form onSubmit={submit}>
        <label>用户名<input name="username" autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} required /></label>
        {mode === "register" && <><p className="account-help">3–32 个 ASCII 字符，以字母开头，可含数字、点、下划线和连字符；忽略首尾空白及大小写。</p>
          <label>邮箱<input name="email" type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} required /></label>
          <p className="account-help">必填且唯一。邮箱未验证，本轮不发送邮件，不用于登录或找回密码。</p>
          <label>显示名称（可选）<input name="display_name" autoComplete="nickname" value={displayName} onChange={(event) => setDisplayName(event.target.value)} /></label>
          <p className="account-help">省略时使用用户名；填写时为 1–64 个字符，不含控制字符。</p></>}
        <label>密码<input name="password" type="password" autoComplete={mode === "register" ? "new-password" : "current-password"} value={password} onChange={(event) => setPassword(event.target.value)} required /></label>
        <p className="account-help">15–128 个字符；空格和大小写都会保留。</p>
        <Button type="submit" variant="primary" disabled={busy || !csrf}>{busy ? "正在处理…" : mode === "register" ? "注册并登录" : "登录"}</Button>
        <p className="account-link">{mode === "register" ? <Link to="/login">已有账号，去登录</Link> : <Link to="/register">注册新账号</Link>}</p>
      </form>}
    </section>
  </main>;
}
