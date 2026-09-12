// Diagnostic transport: retain actual persistent browser profiles across launches.
const { createServer } = require('node:http');
const { dirname, join } = require('node:path');
const core = require(join(dirname(require.resolve('playwright-core/package.json', {paths:[dirname(require.resolve(process.env.RCC_PLAYWRIGHT_PATH))]})), 'lib/coreBundle.js'));
const { createPlaywright, nullProgress } = core.server;
const { PlaywrightServer } = core.remote;
const { SocksProxy } = core.utils;
const servers = new Map();
createServer(async (req, res) => {
  try {
    let body = ''; for await (const chunk of req) body += chunk;
    const { slot, profile, action } = JSON.parse(body);
    if (!['persistent', 'reviewer'].includes(slot)) throw new Error('Unknown diagnostic slot');
    await servers.get(slot)?.close();
    if (action === 'close') { servers.delete(slot); res.end('{}'); return; }
    const socks = new SocksProxy(); socks.setPattern('<loopback>');
    const options = {headless:true, socksProxyPort:await socks.listen(0)};
    const playwright = createPlaywright({sdkLanguage:'javascript',isServer:true});
    const browser = profile
      ? (await playwright.webkit.launchPersistentContext(nullProgress, `/tmp/${profile}`, options))._browser
      : await playwright.webkit.launch(nullProgress, options);
    const remote = new PlaywrightServer({mode:'launchServerShared',path:'/browser',maxConnections:1,preLaunchedBrowser:browser,preLaunchedSocksProxy:socks});
    const endpoint = await remote.listen(slot === 'persistent' ? 47001 : 47002, '0.0.0.0');
    const server = {close:async()=>{await browser.close(nullProgress, {reason:'diagnostic context close'});await remote.close();socks.close();}};
    servers.set(slot, server);
    browser.options.browserProcess.process.on('exit', (code, signal) => console.log(JSON.stringify({ event: 'browser-exit', slot, code, signal, at: Date.now() })));
    console.log(JSON.stringify({ event: 'browser-launch', slot, profile, at: Date.now() }));
    res.end(JSON.stringify({ path: new URL(endpoint).pathname }));
  } catch (error) { res.statusCode = 500; res.end(JSON.stringify({ error: String(error) })); }
}).listen(47000, '0.0.0.0');
