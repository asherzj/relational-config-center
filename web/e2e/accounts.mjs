import assert from 'node:assert/strict';
import { chromium } from 'playwright';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';

const origin = process.env.RCC_E2E_ORIGIN;
assert.ok(origin, 'Run make test-browser to create isolated real services');
const profile = await mkdtemp(join(tmpdir(), 'rcc-account-browser-'));
const launchOptions = process.env.RCC_BROWSER_EXECUTABLE
  ? { executablePath: process.env.RCC_BROWSER_EXECUTABLE, headless: true }
  : { channel: 'chrome', headless: true };
let context;
try {
  context = await chromium.launchPersistentContext(profile, launchOptions);
  let page = await context.newPage();
  const seenURLs = [];
  context.on('request', request => seenURLs.push(request.url()));
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  await page.getByRole('link', { name: '注册新账号' }).click();
  await page.getByLabel('用户名', { exact: true }).fill('browser.user');
  await page.getByLabel('邮箱', { exact: true }).fill('browser.secret@example.com');
  await page.getByLabel('密码', { exact: true }).fill('browser password long enough');
  await page.getByRole('button', { name: '注册并登录' }).click();
  await page.waitForURL(`${origin}/configuration/managed-data`);
  const identity = await page.evaluate(async () => (await fetch('/api/v1/auth/session')).json());
  assert.match(identity.account.id, /^[a-f0-9-]{36}$/);
  assert.equal(identity.account.email_verified, false);
  await page.reload();
  await page.getByRole('heading', { name: '配置内容管理' }).waitFor();
  await context.close();
  context = await chromium.launchPersistentContext(profile, launchOptions);
  context.on('request', request => seenURLs.push(request.url()));
  page = await context.newPage();
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '配置内容管理' }).waitFor();
  await page.getByLabel('Managed Table', { exact: true }).selectOption('notification_templates');
  await page.getByRole('button', { name: '新增记录', exact: true }).click();
  for (const [field, value] of Object.entries({template_key:'browser_system',channel:'PUSH',body:'browser system configuration'})) {
    await page.getByLabel(`包含 ${field}`, { exact: true }).check();
    await page.getByLabel(`${field} 值`, { exact: true }).fill(value);
  }
  await page.getByRole('button', { name: '查看 Change Set' }).click();
  await page.getByRole('button', { name: '确认并执行' }).click();
  await page.getByRole('heading', { name: 'ADD 已完成' }).waitFor();
  const cookies = await context.cookies();
  const session = cookies.find(cookie => cookie.name === 'rcc-session-dev');
  assert.ok(session?.httpOnly && !session.secure && session.sameSite === 'Lax' && session.path === '/');
  const storage = await page.evaluate(async () => ({local:{...localStorage},session:{...sessionStorage},databases:await indexedDB.databases(),cookie:document.cookie}));
  const materials = ['browser.secret@example.com','browser password long enough','browser system configuration',identity.csrf_token,session.value];
  for (const secret of materials) {
    assert.ok(!JSON.stringify(storage).includes(secret), 'sensitive material persisted or script-readable');
    assert.ok(seenURLs.every(url => !decodeURIComponent(url).includes(secret)), 'sensitive material in URL');
  }
  assert.ok(seenURLs.filter(url => new URL(url).pathname.startsWith('/api/')).every(url => new URL(url).origin === origin));
  await page.goto(`${origin}/account`);
  await page.getByRole('button', { name: '退出当前账号' }).click();
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  assert.equal(await page.evaluate(async () => (await fetch('/api/v1/table-policies')).status),401);
  await page.goto(`${origin}/configuration/managed-data`);
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  process.stdout.write(JSON.stringify({account_id:identity.account.id,template_key:'browser_system',checks:['registration','refresh','reopen','business write','HTTP Cookie','storage/URL secrecy','logout rejection']}));
} finally {
  await context?.close();
  await rm(profile, { recursive: true, force: true });
}
