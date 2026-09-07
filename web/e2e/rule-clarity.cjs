const fs = require("node:fs");
const path = require("node:path");
const { browserOptions, registerFixtureAccount } = require("./local-account.cjs");

const playwrightModule = process.env.RCC_PLAYWRIGHT_MODULE || "playwright";
const { chromium } = require(playwrightModule);
const baseURL = process.env.RCC_WEB_URL || "http://127.0.0.1:15173";
const outputDir = process.env.RCC_E2E_OUTPUT || "/tmp/rcc-rule-clarity-browser";
const screenshotPath = path.join(outputDir, "rule-clarity-390.png");
const resultPath = path.join(outputDir, "result.json");

function check(condition, message) {
  if (!condition) throw new Error(message);
}

(async () => {
  fs.mkdirSync(outputDir, { recursive: true });
  let browser;
  let browserVersion = null;
  let page;
  let context;
  let failure = null;
  const pageErrors = [];
  const checks = [];
  const verify = async (name, run) => {
    await run();
    checks.push(name);
  };

  try {
    browser = await chromium.launch(browserOptions());
    browserVersion = browser.version();
    context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    page = await context.newPage();
    const account = await registerFixtureAccount(context, baseURL);
    page.setDefaultTimeout(10000);
    page.setDefaultNavigationTimeout(15000);
    page.on("pageerror", (error) => pageErrors.push(error.message));
    await verify("query detail explains default order and pagination", async () => {
      await page.goto(`${baseURL}/platform/query-policies/notification_page_query_v1`);
      const effect = page.getByRole("region", { name: "实际查询效果" });
      await effect.waitFor();
      const text = await effect.innerText();
      check(text.includes("按 id 降序排列"), "query direction was not explained");
      check(text.includes("默认每页数量为 20"), "default page size was not explained");
      check(text.includes("不能超过 200"), "maximum page size was not explained");
      check(text.includes("没有配置字段白名单"), "query field boundary was not explained");
    });

    await verify("metadata editing is separate from execution rules", async () => {
      await page.goto(`${baseURL}/platform/query-policies/notification_page_query_v1?mode=metadata`);
      await page.getByRole("heading", { name: "修改查询规则名称和描述" }).waitFor();
      check(await page.getByLabel("显示名称").isEnabled(), "display name should be editable");
      check(await page.getByLabel("默认排序字段").isDisabled(), "execution rule should stay locked");
      check(await page.getByText("执行内容不会改变", { exact: false }).isVisible(), "metadata consequence was not explained");
    });

    await verify("deprecated mutation remains effective for its existing assignment", async () => {
      await page.goto(`${baseURL}/platform/mutation-policies/stage1_mutation_v1`);
      const effect = page.getByRole("region", { name: "实际变更效果" });
      await effect.waitFor();
      const text = await effect.innerText();
      check(text.includes("新增：规则允许") && text.includes("修改：规则允许") && text.includes("删除：规则允许"), "mutation permissions were not explained");
      check(text.includes("created_by（当前账号的永久 Account ID）"), "ADD operator Auto Fill was not explained");
      check(text.includes("updated_by（当前账号的永久 Account ID）"), "MODIFY operator Auto Fill was not explained");
      check(text.includes("操作人字段填写当前登录账号的永久 Account ID；历史值保持原样。"), "Operator source was not explained");
      check(await page.getByText("对已有分配仍然有效，但不能用于新分配", { exact: false }).isVisible(), "Deprecated lifecycle behavior was not explained");
    });

    await verify("table detail and replacement preview show selected effects", async () => {
      await page.goto(`${baseURL}/platform/table-policies/stage1_acceptance_items`);
      const current = page.getByRole("region", { name: "当前已选规则效果" });
      await current.waitFor();
      check((await current.innerText()).includes("新增：规则允许"), "current mutation effect is missing");
      await page.getByRole("button", { name: "替换所选规则" }).click();
      const mutationSelect = page.getByRole("combobox", { name: "Active 变更规则" });
      await mutationSelect.waitFor();
      check(await mutationSelect.inputValue() === "stage1_mutation_v1", "existing Deprecated code was changed silently");
      const preview = page.getByRole("region", { name: "所选规则效果预览" });
      check((await preview.innerText()).includes("下一次数据请求立即按所选规则执行"), "replacement preview consequence is missing");
      check((await preview.innerText()).includes("新增：规则允许"), "selected mutation preview is missing");
    });

    await verify("managed data explains live schema and current capabilities", async () => {
      await page.goto(`${baseURL}/configuration/managed-data`);
      const select = page.getByRole("combobox", { name: "Managed Table" });
      await select.waitFor();
      await select.selectOption("stage1_acceptance_items");
      const abilities = page.getByRole("region", { name: "当前表规则能力" });
      await abilities.getByText("本次实时表结构确认了", { exact: false }).waitFor();
      const text = await abilities.innerText();
      check(text.includes("按 id 降序排列"), "managed query effect is missing");
      check(text.includes("新增：规则允许"), "managed mutation effect is missing");
      check(text.includes("created_by") && text.includes("updated_at"), "managed Auto Fill columns are missing");
      check(text.includes("本次实时表结构确认了"), "live schema explanation is missing");
    });

    await verify("390px layout has no page overflow and remains readable", async () => {
      await page.setViewportSize({ width: 390, height: 844 });
      await page.goto(`${baseURL}/platform/table-policies/stage1_acceptance_items`);
      await page.getByRole("region", { name: "当前已选规则效果" }).waitFor();
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      check(overflow <= 1, `page overflowed horizontally by ${overflow}px`);
      await page.screenshot({ path: screenshotPath, fullPage: true, mask: [page.locator('.operator')] });
    });

    await account.assertMemoryOnly(page);
    check(pageErrors.length === 0, `browser page errors: ${pageErrors.join("; ")}`);
    console.log(JSON.stringify({ ok: true, checks: checks.length, resultPath, screenshotPath }));
  } catch (error) {
    failure = { name: error.name, message: error.message, stack: error.stack };
    if (page) {
      await page.screenshot({ path: path.join(outputDir, "failure.png"), fullPage: true, mask: [page.locator(".operator")] }).catch(() => {});
    }
    throw error;
  } finally {
    if (browser) await browser.close().catch(() => {});
    fs.writeFileSync(resultPath, JSON.stringify({
      ok: failure === null,
      baseURL,
      chromium: browserVersion,
      checks,
      pageErrors,
      failure,
    }, null, 2) + "\n");
  }
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
