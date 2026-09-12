if (process.env.RCC_E2E_ENGINE === 'webkit' && process.argv[1]?.endsWith('/accounts.mjs')) {
  const { basename } = require('node:path');
  const playwright = require(process.env.RCC_PLAYWRIGHT_PATH);
  const connect = async (slot, profile) => {
    const response = await fetch(process.env.RCC_WEBKIT_BROKER, { method: 'POST', body: JSON.stringify({slot, profile}) });
    if (!response.ok) throw new Error(await response.text());
    const { path } = await response.json();
    const port = slot === 'persistent' ? process.env.RCC_WEBKIT_PERSISTENT_PORT : process.env.RCC_WEBKIT_REVIEWER_PORT;
    return playwright.webkit.connect(`ws://127.0.0.1:${port}${path}`, { exposeNetwork: '<loopback>' });
  };
  playwright.webkit.launch = () => connect('reviewer');
  playwright.webkit.launchPersistentContext = async profile => {
    const browser = await connect('persistent', basename(profile));
    const context = browser.contexts()[0];
    if (!context) throw new Error('Remote browser did not expose its persistent context');
    context.close = async () => {
      const response = await fetch(process.env.RCC_WEBKIT_BROKER, { method: 'POST', body: JSON.stringify({slot:'persistent',action:'close'}) });
      if (!response.ok) throw new Error(await response.text());
      await browser.close();
    };
    return context;
  };
}
