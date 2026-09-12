// Diagnostic transport only: business requests, script, fixtures and assertions stay unchanged.
if (process.env.RCC_E2E_ENGINE === 'webkit' && process.argv[1]?.endsWith('/browser-accessibility.cjs')) {
  const playwright = require('/Users/asher/Projects/relational-config-center/.worktrees/release-template-browser-ci/web/node_modules/playwright');
  playwright.webkit.launch = () => playwright.webkit.connect(process.env.RCC_WEBKIT_WS_ENDPOINT, { exposeNetwork: '<loopback>' });
  console.log('Linux WebKit transport: official playwright:v1.62.1-noble; client loopback forwarded by Playwright');
}
