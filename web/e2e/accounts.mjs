import { execFileSync } from 'node:child_process';
import assert from 'node:assert/strict';
import { chromium, request } from 'playwright';
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
  assert.deepEqual(identity.account.roles, ['VIEWER']);
  await page.getByRole('button', {name:'新增记录',exact:true}).waitFor();
  assert.equal(await page.getByRole('button', {name:'新增记录',exact:true}).isDisabled(),true);
  assert.ok(process.env.RCC_ACCOUNT_MAINTAIN, 'isolated fixture must supply the maintenance executable');
  execFileSync(process.env.RCC_ACCOUNT_MAINTAIN,['grant-admin','--id',identity.account.id],{stdio:'pipe'});
  // A separate registered account remains VIEWER until the administrator uses the UI.
  const member=await request.newContext();
  const prepared=await member.get(`${origin}/api/v1/auth/csrf`);
  const preparation=await prepared.json();
  const response=await member.post(`${origin}/api/v1/auth/register`,{headers:{Origin:origin,'X-CSRF-Token':preparation.csrf_token},data:{username:'roles.member',email:'roles.member@example.invalid',password:'member password long enough'}});
  assert.equal(response.status(),201);
  assert.deepEqual((await response.json()).account.roles,['VIEWER']);
  await page.goto(`${origin}/platform/account-roles`);
  await page.getByRole('button',{name:'管理 roles.member 的角色'}).waitFor();
  if(process.env.RCC_E2E_OUTPUT)await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'account-roles-desktop.png')});
  await page.setViewportSize({width:390,height:844});
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'role catalog overflows mobile viewport');
  await page.getByRole('button',{name:'管理 roles.member 的角色'}).click();
  await page.getByRole('dialog',{name:'管理 roles.member 的角色'}).evaluate(async element=>{await Promise.all(element.getAnimations({subtree:true}).map(animation=>animation.finished));});
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'role drawer overflows mobile viewport');
  if(process.env.RCC_E2E_OUTPUT)await page.screenshot({path:join(process.env.RCC_E2E_OUTPUT,'account-roles-mobile.png')});
  await page.setViewportSize({width:1280,height:900});
  await page.getByRole('checkbox',{name:/查看者 VIEWER/}).uncheck();
  await page.getByRole('checkbox',{name:/编辑者 EDITOR/}).check();
  await page.getByRole('checkbox',{name:/审批人 APPROVER/}).check();
  await page.getByRole('button',{name:'保存角色',exact:true}).click();
  await page.getByText('角色已保存，后续请求立即生效。').waitFor();
  const memberIdentity=await (await member.get(`${origin}/api/v1/auth/session`)).json();
  assert.deepEqual(memberIdentity.account.roles,['EDITOR','APPROVER']);
  await page.getByRole('button',{name:'管理 roles.member 的角色'}).click();
  await page.getByRole('heading',{name:'角色变更历史'}).waitFor();
  await page.getByText(`操作者：${identity.account.id}`,{exact:true}).waitFor();
  await page.getByRole('button',{name:'关闭',exact:true}).last().click();
  await member.dispose();
  await page.goto(`${origin}/configuration/managed-data`);

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
  // Both branches meet here: a dirty drawer must protect ordinary navigation,
  // while session loss must suspend its prompt and keyboard trap for re-login.
  await page.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('alertdialog', { name: '放弃未保存的修改？' }).waitFor();
  await context.clearCookies();
  await page.evaluate(() => document.dispatchEvent(new Event('visibilitychange')));
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor();
  assert.equal(await page.getByRole('alertdialog').count(), 0);
  await page.getByLabel('用户名', { exact: true }).fill('browser.user');
  await page.keyboard.press('Tab');
  assert.equal(await page.getByLabel('密码', { exact: true }).evaluate(element => element === document.activeElement), true);
  await page.getByLabel('密码', { exact: true }).fill('browser password long enough');
  await page.getByRole('button', { name: '登录', exact: true }).click();
  await page.getByRole('heading', { name: '登录本地账号' }).waitFor({ state: 'hidden' });
  assert.equal(await page.getByLabel('body 值', { exact: true }).inputValue(), 'browser system configuration');
  await page.getByRole('button', { name: '关闭', exact: true }).click();
  await page.getByRole('alertdialog', { name: '放弃未保存的修改？' }).waitFor();
  await page.getByRole('button', { name: '继续编辑', exact: true }).click();
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
  process.stdout.write(JSON.stringify({account_id:identity.account.id,template_key:'browser_system',checks:['registration defaults VIEWER','maintenance ADMIN bootstrap','UI combination grant','390px role catalog and drawer','role history with permanent actor','original member session refresh','refresh','reopen','dirty navigation protection','same-account draft recovery after session loss','hidden drawer keyboard isolation','business write','HTTP Cookie','storage/URL secrecy','logout rejection']}));
} finally {
  await context?.close();
  await rm(profile, { recursive: true, force: true });
}
