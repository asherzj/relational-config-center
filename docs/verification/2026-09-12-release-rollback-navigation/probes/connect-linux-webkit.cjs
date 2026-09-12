// Historical local transport only. This file is not loaded by CI or production.
if (process.env.RCC_E2E_ENGINE === 'webkit' && process.argv[1]?.endsWith('/release-rollbacks.cjs')) {
  const playwright = require(process.env.RCC_PLAYWRIGHT_PATH);
  playwright.webkit.launch = () => playwright.webkit.connect(process.env.RCC_WEBKIT_WS_ENDPOINT, { exposeNetwork: '<loopback>' });
  console.log('Linux WebKit official v1.62.1-noble, loopback forwarded');
}
