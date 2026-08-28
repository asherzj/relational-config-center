const app = document.querySelector("#app");
const toast = document.querySelector("#toast");

const icons = {
  database: '<ellipse cx="12" cy="5" rx="7" ry="3"/><path d="M5 5v6c0 1.7 3.1 3 7 3s7-1.3 7-3V5"/><path d="M5 11v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6"/>',
  layers: '<path d="m12 3-9 5 9 5 9-5-9-5Z"/><path d="m3 12 9 5 9-5"/><path d="m3 16 9 5 9-5"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3A1.7 1.7 0 0 0 10 3V2.8h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/>',
  chevronDown: '<path d="m6 9 6 6 6-6"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  refresh: '<path d="M20 6v5h-5"/><path d="M4 18v-5h5"/><path d="M18.7 9A7 7 0 0 0 6 6.3L4 8m16 8-2 1.7A7 7 0 0 1 5.3 15"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>',
  menu: '<path d="M4 7h16M4 12h16M4 17h16"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5M12 8h.01"/>',
  trash: '<path d="M4 7h16M9 7V4h6v3m3 0-1 13H7L6 7M10 11v5M14 11v5"/>',
  filter: '<path d="M4 5h16l-6 7v5l-4 2v-7L4 5Z"/>',
};

const icon = (name, className = "") =>
  `<svg class="icon ${className}" viewBox="0 0 24 24" aria-hidden="true">${icons[name]}</svg>`;

let policies = [
  {
    table_name: "payment_channels",
    query_policy_code: "standard_page_query_v1",
    mutation_policy_code: "standard_mutation_v1",
    enabled: true,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-20 10:15:30",
    gmt_modified: "2026-08-24 11:42:18",
  },
  {
    table_name: "feature_flags",
    query_policy_code: "standard_page_query_v1",
    mutation_policy_code: "standard_mutation_v1",
    enabled: true,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-21 09:33:21",
    gmt_modified: "2026-08-24 11:58:07",
  },
  {
    table_name: "regional_limits",
    query_policy_code: "legacy_page_query_v1",
    mutation_policy_code: "legacy_mutation_v1",
    enabled: false,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-19 16:05:11",
    gmt_modified: "2026-08-20 08:22:14",
  },
];

let queryPolicies = [
  { code: "standard_page_query_v1", name: "标准分页查询", description: "适用于多数配置表的默认分页规则", type_code: "page_query", default_order_field: "id", default_order_direction: "DESC", default_page_size: 20, max_page_size: 200, status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-22 09:12:08", gmt_modified: "2026-08-23 14:26:11" },
  { code: "compact_page_query_v1", name: "紧凑分页查询", description: "小页读取，用于高频浏览", type_code: "page_query", default_order_field: "id", default_order_direction: "DESC", default_page_size: 10, max_page_size: 50, status: "DRAFT", creator: "local-admin", modifier: "local-admin", gmt_created: "2026-08-24 10:08:31", gmt_modified: "2026-08-24 10:08:31" },
  { code: "legacy_page_query_v1", name: "旧版分页查询", description: "仅供既有表规则继续执行", type_code: "page_query", default_order_field: "id", default_order_direction: "ASC", default_page_size: 20, max_page_size: 100, status: "DEPRECATED", creator: "admin", modifier: "admin", gmt_created: "2026-08-18 11:44:19", gmt_modified: "2026-08-24 09:01:26" },
];

let mutationPolicies = [
  { code: "standard_mutation_v1", name: "标准单表变更", description: "允许新增和修改，并维护标准审计字段", type_code: "single_table_mutation", allow_add: true, allow_modify: true, allow_delete: false, create_operator_field: "creator", create_time_field: "created_at", modify_operator_field: "modifier", modify_time_field: "updated_at", status: "ACTIVE", creator: "admin", modifier: "admin", gmt_created: "2026-08-22 09:18:08", gmt_modified: "2026-08-23 14:31:11" },
  { code: "readonly_mutation_v1", name: "只读变更规则", description: "显式禁止 ADD / MODIFY / DELETE", type_code: "single_table_mutation", allow_add: false, allow_modify: false, allow_delete: false, create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null, status: "DRAFT", creator: "local-admin", modifier: "local-admin", gmt_created: "2026-08-24 10:18:31", gmt_modified: "2026-08-24 10:18:31" },
  { code: "legacy_mutation_v1", name: "旧版单表变更", description: "仅供既有表规则继续执行", type_code: "single_table_mutation", allow_add: true, allow_modify: true, allow_delete: true, create_operator_field: "creator", create_time_field: "created_at", modify_operator_field: "modifier", modify_time_field: "updated_at", status: "DEPRECATED", creator: "admin", modifier: "admin", gmt_created: "2026-08-18 11:48:19", gmt_modified: "2026-08-24 09:06:26" },
];

const emptyPolicyFilters = () => ({
  tableName: "",
  queryPolicy: "",
  mutationPolicy: "",
  enabled: "",
  creator: "",
  modifier: "",
  createdFrom: "",
  createdTo: "",
  modifiedFrom: "",
  modifiedTo: "",
});

const managedTables = {
  feature_flags: {
    label: "功能开关",
    columns: ["id", "code", "enabled", "rollout_percent", "updated_at"],
    rows: [
      { id: "42", code: "checkout_v2", enabled: true, rollout_percent: "25", updated_at: "2026-08-24 11:58:07" },
      { id: "43", code: "search_boost", enabled: true, rollout_percent: "80", updated_at: "2026-08-24 11:42:18" },
      { id: "44", code: "dark_header", enabled: false, rollout_percent: "0", updated_at: "2026-08-23 16:05:11" },
    ],
  },
  payment_channels: {
    label: "支付渠道",
    columns: ["id", "channel_code", "provider", "status", "updated_at"],
    rows: [
      { id: "101", channel_code: "bank_card", provider: "UnionPay", status: "ACTIVE", updated_at: "2026-08-24 10:22:31" },
      { id: "102", channel_code: "digital_wallet", provider: "WalletHub", status: "ACTIVE", updated_at: "2026-08-23 18:07:45" },
      { id: "103", channel_code: "bank_transfer", provider: "FastBank", status: "PAUSED", updated_at: "2026-08-22 09:40:16" },
    ],
  },
  regional_limits: {
    label: "区域限额",
    columns: ["id", "region", "daily_limit", "enabled", "updated_at"],
    rows: [],
  },
};

const state = {
  page: "policies",
  mobileNavOpen: false,
  policyDrawer: { open: false, mode: "view", tableName: "feature_flags" },
  queryPolicyDrawer: { open: false, mode: "view", code: "standard_page_query_v1" },
  mutationPolicyDrawer: { open: false, mode: "view", code: "standard_mutation_v1" },
  policyFilterDraft: emptyPolicyFilters(),
  policyFilters: emptyPolicyFilters(),
  policyQueryLoading: false,
  contentDrawer: { open: false, mode: "edit", rowID: null },
  tablePickerOpen: false,
  selectedTable: "feature_flags",
  tableSearch: "",
  conditions: [],
  queryLoading: false,
  filteredRows: null,
  confirmDelete: null,
  policyError: "",
  queryPolicyError: "",
  mutationPolicyError: "",
  contentError: "",
};

let toastTimer;

function escapeHTML(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

function showToast(message, type = "success") {
  clearTimeout(toastTimer);
  toast.className = `toast show ${type}`;
  toast.innerHTML = `${icon(type === "success" ? "check" : "info")}<span>${escapeHTML(message)}</span>`;
  toastTimer = setTimeout(() => (toast.className = "toast"), 2600);
}

function nowText() {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", second: "2-digit", hour12: false,
  }).format(new Date()).replaceAll("/", "-");
}

function policyFor(tableName) {
  return policies.find((policy) => policy.table_name === tableName);
}

function mutationPolicyForAssignment(policy) {
  return mutationPolicyFor(policy.mutation_policy_code);
}

function navigationMarkup() {
  const platformActive = ["policies", "query-policies", "mutation-policies"].includes(state.page);
  const configActive = state.page === "content";
  return `
    <aside class="sidebar ${state.mobileNavOpen ? "open" : ""}" aria-label="主导航">
      <div class="mobile-brand"><span class="brand-mark">${icon("database")}</span><strong>关系型配置中心</strong><button class="icon-button mobile-nav-close" data-close-nav aria-label="关闭导航">${icon("close")}</button></div>
      <nav class="nav-groups">
        <section class="nav-group ${platformActive ? "expanded" : ""}">
          <button class="nav-primary" data-nav="query-policies" aria-expanded="${platformActive}"><span>${icon("layers")}平台管理</span>${icon("chevronDown", "nav-chevron")}</button>
          <button class="nav-secondary ${state.page === "query-policies" ? "active" : ""}" data-page="query-policies">查询规则定义</button>
          <button class="nav-secondary ${state.page === "mutation-policies" ? "active" : ""}" data-page="mutation-policies">变更规则定义</button>
          <button class="nav-secondary ${state.page === "policies" ? "active" : ""}" data-page="policies">表规则分配</button>
        </section>
        <section class="nav-group ${configActive ? "expanded" : ""}">
          <button class="nav-primary" data-nav="content" aria-expanded="${configActive}"><span>${icon("settings")}配置管理</span>${icon("chevronDown", "nav-chevron")}</button>
          <button class="nav-secondary ${configActive ? "active" : ""}" data-page="content">配置内容管理</button>
        </section>
      </nav>
      <div class="sidebar-foot"><span class="operator-avatar">OP</span><span><strong>local-admin</strong><small>Operator</small></span></div>
    </aside>
    <button class="nav-scrim ${state.mobileNavOpen ? "show" : ""}" data-close-nav aria-label="关闭导航"></button>`;
}

function headerMarkup() {
  return `
    <header class="app-header">
      <div class="header-brand">
        <button class="menu-button" data-toggle-nav aria-label="打开导航">${icon("menu")}</button>
        <span class="brand-mark">${icon("database")}</span><strong>关系型配置中心</strong>
      </div>
      <div class="header-status"><span><i class="ready-dot"></i>MySQL · Ready</span><span class="status-divider"></span><button class="environment">本地环境 ${icon("chevronDown")}</button></div>
    </header>`;
}

function statusMarkup(enabled) {
  return `<span class="status-text ${enabled ? "enabled" : "disabled"}">${enabled ? "启用" : "停用"}</span>`;
}

function capabilityMarkup(allowed) {
  return `<span class="status-text ${allowed ? "enabled" : "disabled"}">${allowed ? "允许" : "禁止"}</span>`;
}

function policyRowsMarkup(visiblePolicies) {
	if (state.policyQueryLoading) return `<tr><td colspan="9"><div class="loading-state policy-loading"><i></i><span>正在查询表规则…</span></div></td></tr>`;
	if (!visiblePolicies.length) return `<tr><td colspan="9"><div class="empty-state policy-empty">${icon("search")}<strong>没有匹配的表规则</strong><span>调整查询条件后重试</span></div></td></tr>`;
  return visiblePolicies.map((policy) => {
    const selected = state.policyDrawer.open && state.policyDrawer.tableName === policy.table_name;
    return `
      <tr class="${selected ? "selected" : ""}" data-policy-row="${policy.table_name}">
		<td><code>${policy.table_name}</code></td><td><code>${policy.query_policy_code}</code></td>
		<td><code>${policy.mutation_policy_code}</code></td>
		<td>${statusMarkup(policy.enabled)}</td><td>${policy.creator}</td><td>${policy.modifier}</td>
		<td class="date-cell">${policy.gmt_created}</td><td class="date-cell">${policy.gmt_modified}</td>
		<td class="action-cell"><button class="link-button" data-policy-action="view" data-table="${policy.table_name}">查看</button><button class="link-button" data-policy-action="edit" data-table="${policy.table_name}">替换</button><button class="link-button ${policy.enabled ? "danger" : ""}" data-policy-action="toggle" data-table="${policy.table_name}">${policy.enabled ? "停用" : "启用"}</button></td>
      </tr>`;
  }).join("");
}

function filteredPolicies() {
  const filters = state.policyFilters;
  const contains = (actual, expected) => !expected || String(actual ?? "").toLowerCase().includes(expected.toLowerCase());
  const withinDates = (actual, from, to) => {
    const date = String(actual).slice(0, 10);
    return (!from || date >= from) && (!to || date <= to);
  };

  return policies.filter((policy) =>
    contains(policy.table_name, filters.tableName)
	&& contains(policy.query_policy_code, filters.queryPolicy)
	&& contains(policy.mutation_policy_code, filters.mutationPolicy)
    && (!filters.enabled || String(policy.enabled) === filters.enabled)
    && contains(policy.creator, filters.creator)
    && contains(policy.modifier, filters.modifier)
	&& withinDates(policy.gmt_created, filters.createdFrom, filters.createdTo)
	&& withinDates(policy.gmt_modified, filters.modifiedFrom, filters.modifiedTo)
  );
}

function policySelectOptions(values, currentValue) {
  return [...new Set(values)].map((value) => `<option value="${escapeHTML(value)}" ${value === currentValue ? "selected" : ""}>${escapeHTML(value)}</option>`).join("");
}

function policyFilterMarkup() {
  const draft = state.policyFilterDraft;
  return `
    <form class="policy-filter-surface" id="policy-filter-form" aria-label="表规则查询条件">
      <div class="policy-filter-grid">
        <label class="policy-filter-field"><span>表名</span><input data-policy-filter name="tableName" class="mono" value="${escapeHTML(draft.tableName)}" placeholder="请输入 table_name"></label>
        <label class="policy-filter-field range-field"><span>创建时间</span><span class="date-range"><input data-policy-filter name="createdFrom" type="date" value="${escapeHTML(draft.createdFrom)}" aria-label="创建开始日期"><i>—</i><input data-policy-filter name="createdTo" type="date" value="${escapeHTML(draft.createdTo)}" aria-label="创建结束日期"></span></label>
        <label class="policy-filter-field range-field"><span>修改时间</span><span class="date-range"><input data-policy-filter name="modifiedFrom" type="date" value="${escapeHTML(draft.modifiedFrom)}" aria-label="修改开始日期"><i>—</i><input data-policy-filter name="modifiedTo" type="date" value="${escapeHTML(draft.modifiedTo)}" aria-label="修改结束日期"></span></label>
        <label class="policy-filter-field"><span>创建人</span><select data-policy-filter name="creator"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.creator), draft.creator)}</select></label>
        <label class="policy-filter-field"><span>修改人</span><select data-policy-filter name="modifier"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.modifier), draft.modifier)}</select></label>
        <label class="policy-filter-field"><span>启用状态</span><select data-policy-filter name="enabled"><option value="">全部</option><option value="true" ${draft.enabled === "true" ? "selected" : ""}>启用</option><option value="false" ${draft.enabled === "false" ? "selected" : ""}>停用</option></select></label>
		<label class="policy-filter-field"><span>查询规则编码</span><select data-policy-filter name="queryPolicy"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.query_policy_code), draft.queryPolicy)}</select></label>
		<label class="policy-filter-field"><span>变更规则编码</span><select data-policy-filter name="mutationPolicy"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.mutation_policy_code), draft.mutationPolicy)}</select></label>
      </div>
      <footer class="policy-filter-actions"><button class="button primary" type="submit" ${state.policyQueryLoading ? "disabled" : ""}>${icon("search")}${state.policyQueryLoading ? "查询中…" : "查询"}</button><button class="button secondary" type="button" data-reset-policy-filters>${icon("refresh")}重置</button></footer>
    </form>`;
}

function policyPageMarkup() {
  const visiblePolicies = filteredPolicies();
  return `
    <main class="workspace">
      <div class="page-head"><div><h1>表规则分配</h1><p>为现有数据库表选择可复用的查询规则与变更规则。</p></div></div>
      ${policyFilterMarkup()}
      <div class="policy-list-toolbar"><button class="button primary" data-new-policy>${icon("plus")}新建表规则</button><button class="button secondary" data-refresh>${icon("refresh")}刷新</button></div>
      <section class="table-surface" aria-label="表规则目录">
		<div class="table-scroll"><table class="data-table policy-table"><thead><tr><th>table_name</th><th>query_policy_code</th><th>mutation_policy_code</th><th>enabled</th><th>creator</th><th>modifier</th><th>gmt_created</th><th>gmt_modified</th><th>操作</th></tr></thead><tbody>${policyRowsMarkup(visiblePolicies)}</tbody></table></div>
        <footer class="table-footer"><span>共 ${visiblePolicies.length} 条表规则${visiblePolicies.length !== policies.length ? ` · 总计 ${policies.length} 条` : ""}</span><span>规则目录 · 实时读取</span></footer>
      </section>
    </main>${policyDrawerMarkup()}`;
}

function lifecycleMarkup(policy) {
	const steps = [["已创建", policy?.gmt_created || "保存后生成"], ["已验证", policy ? policy.gmt_created : "等待校验"], [policy?.enabled ? "已启用" : "待启用", policy?.enabled ? policy.gmt_modified : "—"]];
  return `<ol class="lifecycle">${steps.map(([label, time], index) => `<li class="${index < 2 || policy?.enabled ? "done" : ""}"><i>${icon(index < 2 || policy?.enabled ? "check" : "info")}</i><span><strong>${label}</strong><small>${time}</small></span></li>`).join("")}</ol>`;
}

function policyDrawerMarkup() {
  if (!state.policyDrawer.open) return "";
  const creating = state.policyDrawer.mode === "create";
  const policy = state.policyDrawer.mode === "create" ? null : policyFor(state.policyDrawer.tableName);
  const editing = state.policyDrawer.mode !== "view";
  const title = state.policyDrawer.mode === "create" ? "新建表规则" : state.policyDrawer.mode === "edit" ? "替换表规则" : "表规则详情";
  const submitLabel = state.policyDrawer.mode === "create" ? "创建表规则" : "保存替换";
  const tableName = policy?.table_name || "";
  const enabled = creating ? false : policy?.enabled;
  const queryChoices = editing ? queryPolicies.filter((item) => item.status === "ACTIVE") : queryPolicies.filter((item) => item.code === policy?.query_policy_code);
  const mutationChoices = editing ? mutationPolicies.filter((item) => item.status === "ACTIVE") : mutationPolicies.filter((item) => item.code === policy?.mutation_policy_code);
  return `
    <div class="drawer-layer policy-detail-layer">
      <button class="drawer-backdrop" data-close-policy aria-label="关闭${title}"></button>
      <aside class="detail-drawer policy-detail-drawer" role="dialog" aria-modal="true" aria-label="${title}">
      <header class="drawer-head"><div><span class="drawer-eyebrow">TABLE POLICY</span><h2>${title}</h2></div><button class="icon-button" data-close-policy aria-label="关闭">${icon("close")}</button></header>
      <form id="policy-form" class="drawer-body"><div class="policy-form-grid"><div class="policy-fields">
        <label class="form-field"><span>table_name</span><input name="table_name" class="mono" value="${escapeHTML(tableName)}" ${creating ? "" : "readonly"} ${editing ? "" : "disabled"} placeholder="notification_templates" required></label>
        <label class="form-field"><span>query_policy_code</span><select name="query_policy_code" class="mono" ${editing ? "" : "disabled"}>${queryChoices.map((item) => `<option value="${item.code}" ${item.code === policy?.query_policy_code ? "selected" : ""}>${item.code} · ${escapeHTML(item.name)}</option>`).join("")}</select><small>${editing ? "仅显示 Active Query Policies" : policyStatusMarkup(queryPolicyFor(policy?.query_policy_code)?.status || "ACTIVE")}</small></label>
        <label class="form-field"><span>mutation_policy_code</span><select name="mutation_policy_code" class="mono" ${editing ? "" : "disabled"}>${mutationChoices.map((item) => `<option value="${item.code}" ${item.code === policy?.mutation_policy_code ? "selected" : ""}>${item.code} · ${escapeHTML(item.name)}</option>`).join("")}</select><small>${editing ? "仅显示 Active Mutation Policies" : policyStatusMarkup(mutationPolicyFor(policy?.mutation_policy_code)?.status || "ACTIVE")}</small></label>
        <div class="form-note">${icon("info")}Query 参数、Mutation 权限与 Auto Fill 均来自所选规则定义；表规则不提供 JSON 或每表覆盖。</div>
        <div class="switch-field"><span>enabled</span><label class="switch"><input type="checkbox" name="enabled" ${enabled ? "checked" : ""} disabled><i></i></label><strong>${enabled ? "启用" : "停用"}</strong></div>
        ${policy && !creating ? `<div class="audit-grid"><label class="form-field"><span>creator</span><input value="${policy.creator}" disabled></label><label class="form-field"><span>gmt_created</span><input value="${policy.gmt_created}" disabled></label><label class="form-field"><span>modifier</span><input value="${policy.modifier}" disabled></label><label class="form-field"><span>gmt_modified</span><input value="${policy.gmt_modified}" disabled></label></div>` : ""}
        ${state.policyError ? `<p class="form-error">${icon("info")}${escapeHTML(state.policyError)}</p>` : ""}
      </div>${lifecycleMarkup(creating ? null : policy)}</div></form>
      <footer class="drawer-foot">${editing ? `<button class="button primary" type="submit" form="policy-form">${submitLabel}</button><button class="button secondary" data-close-policy>取消</button>` : `<button class="button primary" data-edit-policy>修改规则</button><button class="button secondary" data-close-policy>关闭</button>`}</footer>
      </aside>
    </div>`;
}

function queryPolicyFor(code) {
  return queryPolicies.find((policy) => policy.code === code);
}

function policyStatusMarkup(status) {
  return `<span class="policy-status ${status.toLowerCase()}">${status}</span>`;
}

function queryPolicyPageMarkup() {
  return `
    <main class="workspace">
      <div class="page-head"><div><h1>查询规则定义</h1><p>创建可复用、版本化的查询规则；Draft 验证通过后才能激活并分配。</p></div><button class="button primary" data-new-query-policy>${icon("plus")}新建 Draft</button></div>
      <section class="catalog-summary" aria-label="查询规则类型注册表"><span>${icon("layers")}已注册规则类型</span><code>page_query</code><small>显式、代码所有 · 无运行时插件</small></section>
      <section class="table-surface" aria-label="查询规则目录">
        <div class="table-scroll"><table class="data-table query-policy-table"><thead><tr><th>规则编码</th><th>名称</th><th>类型</th><th>默认排序</th><th>默认 / 最大页</th><th>状态</th><th>修改人</th><th>gmt_modified</th><th>操作</th></tr></thead><tbody>
          ${queryPolicies.map((policy) => `<tr data-query-policy-row="${policy.code}"><td><code>${policy.code}</code></td><td><strong>${escapeHTML(policy.name)}</strong><small class="cell-description">${escapeHTML(policy.description)}</small></td><td><code>${policy.type_code}</code></td><td><code>${policy.default_order_field} ${policy.default_order_direction}</code></td><td>${policy.default_page_size} / ${policy.max_page_size}</td><td>${policyStatusMarkup(policy.status)}</td><td>${policy.modifier}</td><td class="date-cell">${policy.gmt_modified}</td><td class="action-cell"><button class="link-button" data-query-policy-action="view" data-code="${policy.code}">查看</button>${policy.status === "DRAFT" ? `<button class="link-button" data-query-policy-action="replace" data-code="${policy.code}">编辑</button><button class="link-button" data-query-policy-action="activate" data-code="${policy.code}">激活</button><button class="link-button danger" data-query-policy-action="delete" data-code="${policy.code}">删除</button>` : `<button class="link-button" data-query-policy-action="metadata" data-code="${policy.code}">元数据</button>${policy.status === "ACTIVE" ? `<button class="link-button danger" data-query-policy-action="deprecate" data-code="${policy.code}">弃用</button>` : ""}`}</td></tr>`).join("")}
        </tbody></table></div><footer class="table-footer"><span>共 ${queryPolicies.length} 个查询规则</span><span>规则目录 · 生命周期受保护</span></footer>
      </section>
    </main>${queryPolicyDrawerMarkup()}`;
}

function queryPolicyDrawerMarkup() {
  if (!state.queryPolicyDrawer.open) return "";
  const mode = state.queryPolicyDrawer.mode;
  const creating = mode === "create";
  const policy = creating ? null : queryPolicyFor(state.queryPolicyDrawer.code);
  const editing = ["create", "replace", "metadata"].includes(mode);
  const executionEditable = ["create", "replace"].includes(mode);
  const title = creating ? "新建查询规则 Draft" : mode === "replace" ? "替换 Draft 定义" : mode === "metadata" ? "更新显示元数据" : "查询规则详情";
  const defaults = policy || { code: "", name: "", description: "", type_code: "page_query", default_order_field: "id", default_order_direction: "DESC", default_page_size: 20, max_page_size: 200, status: "DRAFT" };
  const footer = editing
    ? `<button class="button primary" type="submit" form="query-policy-form">${creating ? "创建 Draft" : mode === "replace" ? "保存完整替换" : "保存元数据"}</button><button class="button secondary" data-close-query-policy>取消</button>`
    : `${policy.status === "DRAFT" ? `<button class="button secondary" data-query-policy-action="replace" data-code="${policy.code}">编辑 Draft</button><button class="button primary" data-query-policy-action="activate" data-code="${policy.code}">验证并激活</button><button class="button secondary danger-link" data-query-policy-action="delete" data-code="${policy.code}">删除 Draft</button>` : `<button class="button secondary" data-query-policy-action="metadata" data-code="${policy.code}">更新元数据</button>${policy.status === "ACTIVE" ? `<button class="button secondary danger-link" data-query-policy-action="deprecate" data-code="${policy.code}">弃用</button>` : ""}`}<button class="button secondary" data-close-query-policy>关闭</button>`;
  return `<div class="drawer-layer"><button class="drawer-backdrop" data-close-query-policy aria-label="关闭"></button><aside class="detail-drawer policy-detail-drawer" role="dialog" aria-modal="true" aria-label="${title}">
    <header class="drawer-head"><div><span class="drawer-eyebrow">QUERY POLICY</span><h2>${title}</h2>${policy ? `<p>${policyStatusMarkup(policy.status)}</p>` : ""}</div><button class="icon-button" data-close-query-policy aria-label="关闭">${icon("close")}</button></header>
    <form id="query-policy-form" class="drawer-body typed-policy-form">
      <label class="form-field"><span>规则编码 · 创建后不可变</span><input class="mono" name="code" value="${escapeHTML(defaults.code)}" ${creating ? "" : "readonly"} ${editing ? "" : "disabled"} placeholder="standard_page_query_v2" required></label>
      <label class="form-field"><span>显示名称</span><input name="name" value="${escapeHTML(defaults.name)}" ${editing ? "" : "disabled"} required></label>
      <label class="form-field"><span>描述</span><textarea class="description-input" name="description" ${editing ? "" : "disabled"}>${escapeHTML(defaults.description)}</textarea></label>
      <div class="typed-policy-grid">
        <label class="form-field"><span>规则类型</span><select class="mono" name="type_code" ${executionEditable ? "" : "disabled"}><option value="page_query">page_query</option></select></label>
        <label class="form-field"><span>默认排序字段</span><input class="mono" name="default_order_field" value="${escapeHTML(defaults.default_order_field)}" ${executionEditable ? "" : "disabled"} required></label>
        <label class="form-field"><span>默认排序方向</span><select class="mono" name="default_order_direction" ${executionEditable ? "" : "disabled"}><option ${defaults.default_order_direction === "ASC" ? "selected" : ""}>ASC</option><option ${defaults.default_order_direction === "DESC" ? "selected" : ""}>DESC</option></select></label>
        <label class="form-field"><span>默认页大小</span><input type="number" min="1" max="200" name="default_page_size" value="${defaults.default_page_size}" ${executionEditable ? "" : "disabled"} required></label>
        <label class="form-field"><span>最大页大小 · 上限 200</span><input type="number" min="1" max="200" name="max_page_size" value="${defaults.max_page_size}" ${executionEditable ? "" : "disabled"} required></label>
      </div>
      ${!executionEditable && policy?.status !== "DRAFT" ? `<div class="form-note">${icon("info")}Active / Deprecated 的执行字段已锁定；只能更新名称和描述。</div>` : ""}
      ${policy ? `<div class="audit-grid policy-audit"><label class="form-field"><span>creator</span><input value="${policy.creator}" disabled></label><label class="form-field"><span>gmt_created</span><input value="${policy.gmt_created}" disabled></label><label class="form-field"><span>modifier</span><input value="${policy.modifier}" disabled></label><label class="form-field"><span>gmt_modified</span><input value="${policy.gmt_modified}" disabled></label></div>` : ""}
      ${state.queryPolicyError ? `<p class="form-error">${icon("info")}${escapeHTML(state.queryPolicyError)}</p>` : ""}
    </form><footer class="drawer-foot query-policy-actions">${footer}</footer></aside></div>`;
}

function mutationPolicyFor(code) {
  return mutationPolicies.find((policy) => policy.code === code);
}

function autoFillSummary(policy) {
  const targets = [policy.create_operator_field, policy.create_time_field, policy.modify_operator_field, policy.modify_time_field].filter(Boolean);
  return targets.length ? targets.map((target) => `<code>${escapeHTML(target)}</code>`).join(" ") : '<span class="muted">无</span>';
}

function mutationPolicyPageMarkup() {
  return `
    <main class="workspace">
      <div class="page-head"><div><h1>变更规则定义</h1><p>以关系字段定义操作授权和四个固定 Auto Fill 槽位；不使用配置 JSON。</p></div><button class="button primary" data-new-mutation-policy>${icon("plus")}新建 Draft</button></div>
      <section class="catalog-summary" aria-label="变更规则类型注册表"><span>${icon("layers")}已注册规则类型</span><code>single_table_mutation</code><small>实现 ADD · MODIFY · DELETE</small></section>
      <section class="table-surface" aria-label="变更规则目录">
        <div class="table-scroll"><table class="data-table query-policy-table"><thead><tr><th>规则编码</th><th>名称</th><th>类型</th><th>ADD</th><th>MODIFY</th><th>DELETE</th><th>Auto Fill 目标</th><th>状态</th><th>gmt_modified</th><th>操作</th></tr></thead><tbody>
          ${mutationPolicies.map((policy) => `<tr data-mutation-policy-row="${policy.code}"><td><code>${policy.code}</code></td><td><strong>${escapeHTML(policy.name)}</strong><small class="cell-description">${escapeHTML(policy.description)}</small></td><td><code>${policy.type_code}</code></td><td>${capabilityMarkup(policy.allow_add)}</td><td>${capabilityMarkup(policy.allow_modify)}</td><td>${capabilityMarkup(policy.allow_delete)}</td><td>${autoFillSummary(policy)}</td><td>${policyStatusMarkup(policy.status)}</td><td class="date-cell">${policy.gmt_modified}</td><td class="action-cell"><button class="link-button" data-mutation-policy-action="view" data-code="${policy.code}">查看</button>${policy.status === "DRAFT" ? `<button class="link-button" data-mutation-policy-action="replace" data-code="${policy.code}">编辑</button><button class="link-button" data-mutation-policy-action="activate" data-code="${policy.code}">激活</button><button class="link-button danger" data-mutation-policy-action="delete" data-code="${policy.code}">删除</button>` : `<button class="link-button" data-mutation-policy-action="metadata" data-code="${policy.code}">元数据</button>${policy.status === "ACTIVE" ? `<button class="link-button danger" data-mutation-policy-action="deprecate" data-code="${policy.code}">弃用</button>` : ""}`}</td></tr>`).join("")}
        </tbody></table></div><footer class="table-footer"><span>共 ${mutationPolicies.length} 个变更规则</span><span>固定字段 · 生命周期受保护</span></footer>
      </section>
    </main>${mutationPolicyDrawerMarkup()}`;
}

function mutationPolicyDrawerMarkup() {
  if (!state.mutationPolicyDrawer.open) return "";
  const mode = state.mutationPolicyDrawer.mode;
  const creating = mode === "create";
  const policy = creating ? null : mutationPolicyFor(state.mutationPolicyDrawer.code);
  const editing = ["create", "replace", "metadata"].includes(mode);
  const executionEditable = ["create", "replace"].includes(mode);
  const title = creating ? "新建变更规则 Draft" : mode === "replace" ? "替换 Draft 定义" : mode === "metadata" ? "更新显示元数据" : "变更规则详情";
  const defaults = policy || { code: "", name: "", description: "", type_code: "single_table_mutation", allow_add: false, allow_modify: false, allow_delete: false, create_operator_field: null, create_time_field: null, modify_operator_field: null, modify_time_field: null, status: "DRAFT" };
  const footer = editing
    ? `<button class="button primary" type="submit" form="mutation-policy-form">${creating ? "创建 Draft" : mode === "replace" ? "保存完整替换" : "保存元数据"}</button><button class="button secondary" data-close-mutation-policy>取消</button>`
    : `${policy.status === "DRAFT" ? `<button class="button secondary" data-mutation-policy-action="replace" data-code="${policy.code}">编辑 Draft</button><button class="button primary" data-mutation-policy-action="activate" data-code="${policy.code}">验证并激活</button><button class="button secondary danger-link" data-mutation-policy-action="delete" data-code="${policy.code}">删除 Draft</button>` : `<button class="button secondary" data-mutation-policy-action="metadata" data-code="${policy.code}">更新元数据</button>${policy.status === "ACTIVE" ? `<button class="button secondary danger-link" data-mutation-policy-action="deprecate" data-code="${policy.code}">弃用</button>` : ""}`}<button class="button secondary" data-close-mutation-policy>关闭</button>`;
  return `<div class="drawer-layer"><button class="drawer-backdrop" data-close-mutation-policy aria-label="关闭"></button><aside class="detail-drawer policy-detail-drawer" role="dialog" aria-modal="true" aria-label="${title}">
    <header class="drawer-head"><div><span class="drawer-eyebrow">MUTATION POLICY</span><h2>${title}</h2>${policy ? `<p>${policyStatusMarkup(policy.status)}</p>` : ""}</div><button class="icon-button" data-close-mutation-policy aria-label="关闭">${icon("close")}</button></header>
    <form id="mutation-policy-form" class="drawer-body typed-policy-form">
      <label class="form-field"><span>规则编码 · 创建后不可变</span><input class="mono" name="code" value="${escapeHTML(defaults.code)}" ${creating ? "" : "readonly"} ${editing ? "" : "disabled"} placeholder="standard_mutation_v2" required></label>
      <label class="form-field"><span>显示名称</span><input name="name" value="${escapeHTML(defaults.name)}" ${editing ? "" : "disabled"} required></label>
      <label class="form-field"><span>描述</span><textarea class="description-input" name="description" ${editing ? "" : "disabled"}>${escapeHTML(defaults.description)}</textarea></label>
      <label class="form-field"><span>规则类型</span><select class="mono" name="type_code" ${executionEditable ? "" : "disabled"}><option value="single_table_mutation">single_table_mutation</option></select></label>
      <section class="typed-policy-section"><h3>操作授权</h3><div class="capability-grid">
        <label class="switch-field"><span>allow_add</span><label class="switch"><input type="checkbox" name="allow_add" ${defaults.allow_add ? "checked" : ""} ${executionEditable ? "" : "disabled"}><i></i></label><strong>${defaults.allow_add ? "允许" : "禁止"}</strong></label>
        <label class="switch-field"><span>allow_modify</span><label class="switch"><input type="checkbox" name="allow_modify" ${defaults.allow_modify ? "checked" : ""} ${executionEditable ? "" : "disabled"}><i></i></label><strong>${defaults.allow_modify ? "允许" : "禁止"}</strong></label>
        <label class="switch-field danger-capability"><span>allow_delete</span><label class="switch"><input type="checkbox" name="allow_delete" ${defaults.allow_delete ? "checked" : ""} ${executionEditable ? "" : "disabled"}><i></i></label><strong>${defaults.allow_delete ? "允许" : "禁止"}</strong></label>
      </div></section>
      <section class="typed-policy-section"><h3>标准 Auto Fill 目标字段</h3><p class="section-help">Operator 使用当前 Operator；Time 使用数据库时间。留空表示不填充。</p><div class="typed-policy-grid">
        <label class="form-field"><span>Create Operator Field</span><input class="mono" name="create_operator_field" value="${escapeHTML(defaults.create_operator_field || "")}" ${executionEditable ? "" : "disabled"} placeholder="creator"></label>
        <label class="form-field"><span>Create Time Field</span><input class="mono" name="create_time_field" value="${escapeHTML(defaults.create_time_field || "")}" ${executionEditable ? "" : "disabled"} placeholder="created_at"></label>
        <label class="form-field"><span>Modify Operator Field</span><input class="mono" name="modify_operator_field" value="${escapeHTML(defaults.modify_operator_field || "")}" ${executionEditable ? "" : "disabled"} placeholder="modifier"></label>
        <label class="form-field"><span>Modify Time Field</span><input class="mono" name="modify_time_field" value="${escapeHTML(defaults.modify_time_field || "")}" ${executionEditable ? "" : "disabled"} placeholder="updated_at"></label>
      </div></section>
      ${!executionEditable && policy?.status !== "DRAFT" ? `<div class="form-note">${icon("info")}Active / Deprecated 的授权与 Auto Fill 字段已锁定。</div>` : ""}
      ${policy ? `<div class="audit-grid policy-audit"><label class="form-field"><span>creator</span><input value="${policy.creator}" disabled></label><label class="form-field"><span>gmt_created</span><input value="${policy.gmt_created}" disabled></label><label class="form-field"><span>modifier</span><input value="${policy.modifier}" disabled></label><label class="form-field"><span>gmt_modified</span><input value="${policy.gmt_modified}" disabled></label></div>` : ""}
      ${state.mutationPolicyError ? `<p class="form-error">${icon("info")}${escapeHTML(state.mutationPolicyError)}</p>` : ""}
    </form><footer class="drawer-foot query-policy-actions">${footer}</footer></aside></div>`;
}

function tablePickerMarkup() {
  const options = policies.filter((policy) => policy.table_name.toLowerCase().includes(state.tableSearch.toLowerCase()));
  return `
    <div class="picker-wrap"><label>选择 Managed Table</label>
      <button class="table-picker ${state.tablePickerOpen ? "open" : ""}" data-toggle-picker aria-expanded="${state.tablePickerOpen}"><span><code>${state.selectedTable}</code><small>${managedTables[state.selectedTable].label}</small></span>${icon("chevronDown")}</button>
      ${state.tablePickerOpen ? `<div class="picker-menu"><div class="picker-search">${icon("search")}<input id="table-search" value="${escapeHTML(state.tableSearch)}" placeholder="搜索表名" autocomplete="off"></div><div class="picker-options">${options.map((policy) => `<button data-select-table="${policy.table_name}" ${policy.enabled ? "" : "disabled"} class="${policy.table_name === state.selectedTable ? "selected" : ""}"><span><code>${policy.table_name}</code><small>${managedTables[policy.table_name]?.label || ""}</small></span>${policy.enabled ? policy.table_name === state.selectedTable ? icon("check") : "" : "<em>已停用</em>"}</button>`).join("") || '<p class="picker-empty">没有匹配的表</p>'}</div></div>` : ""}
    </div>`;
}

function conditionsMarkup(table) {
  return state.conditions.map((condition, index) => `<div class="condition-row"><select data-condition-field="${index}">${table.columns.map((column) => `<option ${condition.field === column ? "selected" : ""}>${column}</option>`).join("")}</select><select data-condition-operator="${index}"><option value="exact" ${condition.operator === "exact" ? "selected" : ""}>exact</option><option value="contains" ${condition.operator === "contains" ? "selected" : ""}>contains</option></select><input data-condition-value="${index}" value="${escapeHTML(condition.value)}" placeholder="输入值"><button class="icon-button" data-remove-condition="${index}" aria-label="移除条件">${icon("close")}</button></div>`).join("");
}

function visibleRows() {
  return state.filteredRows ?? managedTables[state.selectedTable].rows;
}

function cellMarkup(column, value) {
  if (typeof value === "boolean") return `<span class="boolean-value ${value ? "true" : "false"}"><i></i>${value ? "true" : "false"}</span>`;
  if (column === "status") return `<span class="status-text ${value === "ACTIVE" ? "enabled" : "disabled"}">${value}</span>`;
  return `<span class="${["id", "code", "channel_code"].includes(column) ? "mono" : ""}">${escapeHTML(value)}</span>`;
}

function contentRowsMarkup(table, policy) {
  const rows = visibleRows();
	const mutationPolicy = mutationPolicyForAssignment(policy);
  if (state.queryLoading) return `<tr><td colspan="${table.columns.length + 1}"><div class="loading-state"><i></i><span>正在执行查询规则…</span></div></td></tr>`;
  if (!rows.length) return `<tr><td colspan="${table.columns.length + 1}"><div class="empty-state">${icon("search")}<strong>没有匹配的配置内容</strong><span>调整查询条件后重试</span></div></td></tr>`;
	return rows.map((row) => `<tr>${table.columns.map((column) => `<td>${cellMarkup(column, row[column])}</td>`).join("")}<td class="action-cell"><button class="link-button" data-edit-row="${row.id}" ${mutationPolicy.allow_modify ? "" : 'disabled title="变更规则禁止修改"'}>编辑</button><button class="link-button danger" data-delete-row="${row.id}" ${mutationPolicy.allow_delete ? "" : 'disabled title="变更规则禁止删除"'}>删除</button></td></tr>`).join("");
}

function contentPageMarkup() {
  const table = managedTables[state.selectedTable];
  const policy = policyFor(state.selectedTable);
	const mutationPolicy = mutationPolicyForAssignment(policy);
  return `
    <main class="workspace content-workspace ${state.contentDrawer.open ? "with-drawer" : ""}">
      <div class="page-head compact"><div><h1>配置内容管理</h1><p>选择 enabled Managed Table，并按当前规则快照查询或变更内容。</p></div></div>
      <section class="content-controls">${tablePickerMarkup()}<div class="policy-summary"><i class="ready-dot"></i><span>表规则 <strong>已启用</strong></span><b>·</b><code>${policy.query_policy_code}</code><button data-view-current-policy>查看规则</button></div>
        <div class="query-builder"><div class="query-head"><strong>${icon("filter")}查询条件</strong><span>最多 20 个条件，使用 AND 连接</span></div><div class="condition-list">${conditionsMarkup(table)}</div><div class="query-actions"><button class="button dashed" data-add-condition>${icon("plus")}添加条件</button><label><span>排序</span><select id="order-field">${table.columns.map((column) => `<option ${column === "id" ? "selected" : ""}>${column}</option>`).join("")}</select></label><label><span>方向</span><select id="order-direction"><option>DESC</option><option>ASC</option></select></label><button class="button primary" data-query>${icon("search")}查询</button></div></div>
      </section>
	  <section class="table-surface content-table-surface"><div class="table-toolbar"><div><strong>${table.label}</strong><code>${state.selectedTable}</code></div><button class="button primary" data-add-row ${mutationPolicy.allow_add ? "" : "disabled"}>${icon("plus")}新增内容</button></div><div class="table-scroll"><table class="data-table content-table"><thead><tr>${table.columns.map((column) => `<th>${column}</th>`).join("")}<th>操作</th></tr></thead><tbody>${contentRowsMarkup(table, policy)}</tbody></table></div><footer class="table-footer"><span>共 ${visibleRows().length} 条记录</span><span>实时 Schema · JSON String</span></footer></section>
    </main>${contentDrawerMarkup()}${confirmMarkup()}`;
}

function editableColumns() {
  return managedTables[state.selectedTable].columns.filter((column) => !["id", "updated_at"].includes(column));
}

function inputForColumn(column, value) {
  if (column === "enabled") return `<label class="switch-field form-switch"><span>${column}</span><label class="switch"><input type="checkbox" name="${column}" ${value ? "checked" : ""}><i></i></label><strong>${value ? "启用" : "停用"}</strong></label>`;
  const numeric = column.includes("percent") || column.includes("limit");
  return `<label class="form-field"><span>${column}</span><input name="${column}" value="${escapeHTML(value ?? "")}" ${numeric ? 'type="number" min="0"' : ""} required></label>`;
}

function contentDrawerMarkup() {
  if (!state.contentDrawer.open) return "";
  const table = managedTables[state.selectedTable];
  const row = state.contentDrawer.mode === "edit" ? table.rows.find((item) => item.id === state.contentDrawer.rowID) : {};
  const title = state.contentDrawer.mode === "edit" ? `编辑 ${state.selectedTable} #${row.id}` : `新增 ${state.selectedTable}`;
  return `<aside class="detail-drawer content-drawer" role="dialog" aria-label="${title}"><header class="drawer-head"><div><h2>${title}</h2><p>字段来自当前 Managed Table 实时 Schema</p></div><button class="icon-button" data-close-content aria-label="关闭">${icon("close")}</button></header><form id="content-form" class="drawer-body content-form">${editableColumns().map((column) => inputForColumn(column, row?.[column])).join("")}${state.contentError ? `<p class="form-error">${icon("info")}${escapeHTML(state.contentError)}</p>` : ""}<div class="form-note">${icon("info")}服务端 Auto Fill 将覆盖 modifier 与 updated_at。</div></form><footer class="drawer-foot"><button class="button primary" type="submit" form="content-form">${state.contentDrawer.mode === "edit" ? "保存修改" : "创建内容"}</button><button class="button secondary" data-close-content>取消</button></footer></aside>`;
}

function confirmMarkup() {
  if (!state.confirmDelete) return "";
  return `<div class="modal-backdrop"><section class="confirm-dialog" role="alertdialog" aria-modal="true"><i class="confirm-icon">${icon("trash")}</i><h2>删除配置内容？</h2><p>将永久删除 <code>${state.selectedTable} #${state.confirmDelete}</code>，此操作无法撤销。</p><div><button class="button danger-button" data-confirm-delete>确认删除</button><button class="button secondary" data-cancel-delete>取消</button></div></section></div>`;
}

function render() {
  const pageMarkup = state.page === "query-policies" ? queryPolicyPageMarkup() : state.page === "mutation-policies" ? mutationPolicyPageMarkup() : state.page === "policies" ? policyPageMarkup() : contentPageMarkup();
  app.innerHTML = `${headerMarkup()}${navigationMarkup()}${pageMarkup}`;
  document.body.classList.toggle("layer-open", state.policyDrawer.open || state.queryPolicyDrawer.open || state.mutationPolicyDrawer.open);
  bindEvents();
}

function changePage(page) {
  state.page = page;
  state.mobileNavOpen = false;
  state.policyDrawer.open = false;
  state.queryPolicyDrawer.open = false;
  state.mutationPolicyDrawer.open = false;
  state.contentDrawer.open = false;
  state.tablePickerOpen = false;
  render();
}

function bindQueryPolicyEvents() {
  document.querySelector("[data-new-query-policy]")?.addEventListener("click", () => { state.queryPolicyDrawer = { open: true, mode: "create", code: "" }; state.queryPolicyError = ""; render(); });
  document.querySelectorAll("[data-query-policy-row]").forEach((row) => row.addEventListener("click", (event) => { if (event.target.closest("button")) return; state.queryPolicyDrawer = { open: true, mode: "view", code: row.dataset.queryPolicyRow }; render(); }));
  document.querySelectorAll("[data-query-policy-action]").forEach((button) => button.addEventListener("click", () => handleQueryPolicyAction(button.dataset.queryPolicyAction, button.dataset.code)));
  document.querySelectorAll("[data-close-query-policy]").forEach((button) => button.addEventListener("click", (event) => { event.preventDefault(); state.queryPolicyDrawer.open = false; render(); }));
  document.querySelector("#query-policy-form")?.addEventListener("submit", saveQueryPolicy);
}

function handleQueryPolicyAction(action, code) {
  const policy = queryPolicyFor(code);
  state.queryPolicyError = "";
  if (["view", "replace", "metadata"].includes(action)) { state.queryPolicyDrawer = { open: true, mode: action, code }; render(); return; }
  if (action === "activate") {
    try {
      if (policy.status !== "DRAFT") throw new Error("只有 Draft 可以激活");
      if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(policy.default_order_field)) throw new Error("默认排序字段不是安全标识符");
      if (!["ASC", "DESC"].includes(policy.default_order_direction)) throw new Error("排序方向无效");
      if (policy.default_page_size < 1 || policy.max_page_size > 200 || policy.default_page_size > policy.max_page_size) throw new Error("分页大小超出平台安全限制");
      policy.status = "ACTIVE"; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${code} 已激活`); state.queryPolicyDrawer = { open: true, mode: "view", code }; render();
    } catch (error) { state.queryPolicyError = error.message; state.queryPolicyDrawer = { open: true, mode: "view", code }; render(); }
    return;
  }
  if (action === "deprecate" && policy.status === "ACTIVE") { policy.status = "DEPRECATED"; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${code} 已弃用`); state.queryPolicyDrawer = { open: true, mode: "view", code }; render(); return; }
  if (action === "delete" && policy.status === "DRAFT") { queryPolicies = queryPolicies.filter((item) => item.code !== code); state.queryPolicyDrawer.open = false; showToast(`${code} Draft 已删除`); render(); }
}

function saveQueryPolicy(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  try {
    const mode = state.queryPolicyDrawer.mode;
    const code = String(data.get("code")).trim();
    const name = String(data.get("name")).trim();
    if (!name) throw new Error("显示名称不能为空");
    if (mode === "metadata") {
      const policy = queryPolicyFor(state.queryPolicyDrawer.code);
      policy.name = name; policy.description = String(data.get("description")).trim(); policy.modifier = "local-admin"; policy.gmt_modified = nowText(); state.queryPolicyDrawer.mode = "view"; showToast("显示元数据已更新"); render(); return;
    }
    if (!/^[a-z][a-z0-9_]*_v[1-9][0-9]*$/.test(code) || /^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_/.test(code)) throw new Error("规则编码必须小写、带版本且技术中立");
    const candidate = { code, name, description: String(data.get("description")).trim(), type_code: String(data.get("type_code")), default_order_field: String(data.get("default_order_field")).trim(), default_order_direction: String(data.get("default_order_direction")), default_page_size: Number(data.get("default_page_size")), max_page_size: Number(data.get("max_page_size")), status: "DRAFT", creator: "local-admin", modifier: "local-admin", gmt_created: nowText(), gmt_modified: nowText() };
    if (candidate.default_page_size < 1 || candidate.max_page_size < 1 || candidate.default_page_size > candidate.max_page_size || candidate.max_page_size > 200) throw new Error("分页大小必须为正数、默认值不能大于最大值，且最大值不能超过 200");
    if (mode === "create") {
      if (queryPolicyFor(code)) throw new Error("Query namespace 中已存在该规则编码");
      queryPolicies.push(candidate); showToast(`${code} Draft 已创建`);
    } else {
      const current = queryPolicyFor(state.queryPolicyDrawer.code);
      if (current.status !== "DRAFT" || current.code !== code) throw new Error("只能完整替换 Draft，且规则编码不可修改");
      Object.assign(current, candidate, { creator: current.creator, gmt_created: current.gmt_created }); showToast(`${code} Draft 已完整替换`);
    }
    state.queryPolicyDrawer = { open: true, mode: "view", code }; state.queryPolicyError = ""; render();
  } catch (error) { state.queryPolicyError = error.message; render(); }
}

function bindMutationPolicyEvents() {
  document.querySelector("[data-new-mutation-policy]")?.addEventListener("click", () => { state.mutationPolicyDrawer = { open: true, mode: "create", code: "" }; state.mutationPolicyError = ""; render(); });
  document.querySelectorAll("[data-mutation-policy-row]").forEach((row) => row.addEventListener("click", (event) => { if (event.target.closest("button")) return; state.mutationPolicyDrawer = { open: true, mode: "view", code: row.dataset.mutationPolicyRow }; render(); }));
  document.querySelectorAll("[data-mutation-policy-action]").forEach((button) => button.addEventListener("click", () => handleMutationPolicyAction(button.dataset.mutationPolicyAction, button.dataset.code)));
  document.querySelectorAll("[data-close-mutation-policy]").forEach((button) => button.addEventListener("click", (event) => { event.preventDefault(); state.mutationPolicyDrawer.open = false; render(); }));
  document.querySelector("#mutation-policy-form")?.addEventListener("submit", saveMutationPolicy);
  document.querySelectorAll("#mutation-policy-form .switch input").forEach((input) => input.addEventListener("change", () => { input.closest(".switch-field").querySelector("strong").textContent = input.checked ? "允许" : "禁止"; }));
}

function validateMutationPolicy(policy) {
  if (policy.type_code !== "single_table_mutation") throw new Error("未知变更规则类型");
  const targets = [policy.create_operator_field, policy.create_time_field, policy.modify_operator_field, policy.modify_time_field].filter(Boolean);
  if (targets.some((target) => !/^[A-Za-z_][A-Za-z0-9_]*$/.test(target))) throw new Error("Auto Fill 目标必须是安全字段名");
  if (new Set(targets).size !== targets.length) throw new Error("四个 Auto Fill 目标不能重复");
  if (!policy.allow_add && (policy.create_operator_field || policy.create_time_field)) throw new Error("Create Auto Fill 需要允许 ADD");
  if (!policy.allow_add && !policy.allow_modify && (policy.modify_operator_field || policy.modify_time_field)) throw new Error("Modify Auto Fill 需要允许 ADD 或 MODIFY");
}

function handleMutationPolicyAction(action, code) {
  const policy = mutationPolicyFor(code);
  state.mutationPolicyError = "";
  if (["view", "replace", "metadata"].includes(action)) { state.mutationPolicyDrawer = { open: true, mode: action, code }; render(); return; }
  if (action === "activate") {
    try {
      if (policy.status !== "DRAFT") throw new Error("只有 Draft 可以激活");
      validateMutationPolicy(policy);
      policy.status = "ACTIVE"; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${code} 已激活`); state.mutationPolicyDrawer = { open: true, mode: "view", code }; render();
    } catch (error) { state.mutationPolicyError = error.message; state.mutationPolicyDrawer = { open: true, mode: "view", code }; render(); }
    return;
  }
  if (action === "deprecate" && policy.status === "ACTIVE") { policy.status = "DEPRECATED"; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${code} 已弃用`); state.mutationPolicyDrawer = { open: true, mode: "view", code }; render(); return; }
  if (action === "delete" && policy.status === "DRAFT") { mutationPolicies = mutationPolicies.filter((item) => item.code !== code); state.mutationPolicyDrawer.open = false; showToast(`${code} Draft 已删除`); render(); }
}

function saveMutationPolicy(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  try {
    const mode = state.mutationPolicyDrawer.mode;
    const code = String(data.get("code")).trim();
    const name = String(data.get("name")).trim();
    if (!name) throw new Error("显示名称不能为空");
    if (mode === "metadata") {
      const policy = mutationPolicyFor(state.mutationPolicyDrawer.code);
      policy.name = name; policy.description = String(data.get("description")).trim(); policy.modifier = "local-admin"; policy.gmt_modified = nowText(); state.mutationPolicyDrawer.mode = "view"; showToast("显示元数据已更新"); render(); return;
    }
    if (!/^[a-z][a-z0-9_]*_v[1-9][0-9]*$/.test(code) || /^(mysql|mariadb|postgres|postgresql|sqlite|oracle|sqlserver|mongodb|gorm|sql)_/.test(code)) throw new Error("规则编码必须小写、带版本且技术中立");
    const optionalField = (field) => String(data.get(field) || "").trim() || null;
    const candidate = { code, name, description: String(data.get("description")).trim(), type_code: String(data.get("type_code")), allow_add: data.has("allow_add"), allow_modify: data.has("allow_modify"), allow_delete: data.has("allow_delete"), create_operator_field: optionalField("create_operator_field"), create_time_field: optionalField("create_time_field"), modify_operator_field: optionalField("modify_operator_field"), modify_time_field: optionalField("modify_time_field"), status: "DRAFT", creator: "local-admin", modifier: "local-admin", gmt_created: nowText(), gmt_modified: nowText() };
    if (mode === "create") {
      if (mutationPolicyFor(code)) throw new Error("Mutation namespace 中已存在该规则编码");
      mutationPolicies.push(candidate); showToast(`${code} Draft 已创建`);
    } else {
      const current = mutationPolicyFor(state.mutationPolicyDrawer.code);
      if (current.status !== "DRAFT" || current.code !== code) throw new Error("只能完整替换 Draft，且规则编码不可修改");
      Object.assign(current, candidate, { creator: current.creator, gmt_created: current.gmt_created }); showToast(`${code} Draft 已完整替换`);
    }
    state.mutationPolicyDrawer = { open: true, mode: "view", code }; state.mutationPolicyError = ""; render();
  } catch (error) { state.mutationPolicyError = error.message; render(); }
}

function bindNavigation() {
  document.querySelectorAll("[data-page], [data-nav]").forEach((button) => button.addEventListener("click", () => changePage(button.dataset.page || button.dataset.nav)));
  document.querySelector("[data-toggle-nav]")?.addEventListener("click", () => { state.mobileNavOpen = !state.mobileNavOpen; render(); });
  document.querySelectorAll("[data-close-nav]").forEach((button) => button.addEventListener("click", () => { state.mobileNavOpen = false; render(); }));
}

function bindPolicyEvents() {
  const filterForm = document.querySelector("#policy-filter-form");
  filterForm?.querySelectorAll("[data-policy-filter]").forEach((field) => {
    const syncValue = () => { state.policyFilterDraft[field.name] = field.value; };
    field.addEventListener("input", syncValue);
    field.addEventListener("change", syncValue);
  });
  filterForm?.addEventListener("submit", (event) => {
    event.preventDefault();
    state.policyFilterDraft = { ...emptyPolicyFilters(), ...Object.fromEntries(new FormData(event.currentTarget)) };
    if (state.policyFilterDraft.createdFrom && state.policyFilterDraft.createdTo && state.policyFilterDraft.createdFrom > state.policyFilterDraft.createdTo) return showToast("创建开始日期不能晚于结束日期", "error");
    if (state.policyFilterDraft.modifiedFrom && state.policyFilterDraft.modifiedTo && state.policyFilterDraft.modifiedFrom > state.policyFilterDraft.modifiedTo) return showToast("修改开始日期不能晚于结束日期", "error");
    const submittedFilters = { ...state.policyFilterDraft };
    state.policyQueryLoading = true;
    render();
    setTimeout(() => {
      state.policyFilters = submittedFilters;
      state.policyQueryLoading = false;
      const count = filteredPolicies().length;
      showToast(`查询完成，共 ${count} 条表规则`);
      render();
    }, 360);
  });
  document.querySelector("[data-reset-policy-filters]")?.addEventListener("click", () => {
    state.policyFilterDraft = emptyPolicyFilters();
    state.policyFilters = emptyPolicyFilters();
    state.policyQueryLoading = false;
    showToast("查询条件已重置");
    render();
  });
  document.querySelector("[data-new-policy]")?.addEventListener("click", () => { state.policyDrawer = { open: true, mode: "create", tableName: "" }; state.policyError = ""; render(); });
  document.querySelector("[data-refresh]")?.addEventListener("click", (event) => { event.currentTarget.classList.add("spinning"); setTimeout(() => showToast("规则目录已刷新"), 350); });
  document.querySelectorAll("[data-policy-row]").forEach((row) => row.addEventListener("click", (event) => { if (event.target.closest("button")) return; state.policyDrawer = { open: true, mode: "view", tableName: row.dataset.policyRow }; render(); }));
  document.querySelectorAll("[data-policy-action]").forEach((button) => button.addEventListener("click", () => {
    const policy = policyFor(button.dataset.table);
    const action = button.dataset.policyAction;
	if (action === "toggle") { policy.enabled = !policy.enabled; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${policy.table_name} 已${policy.enabled ? "启用" : "停用"}`); render(); return; }
    state.policyDrawer = { open: true, mode: action, tableName: policy.table_name }; state.policyError = ""; render();
  }));
  document.querySelectorAll("[data-close-policy]").forEach((button) => button.addEventListener("click", (event) => { event.preventDefault(); state.policyDrawer.open = false; render(); }));
  document.querySelector("[data-edit-policy]")?.addEventListener("click", () => { state.policyDrawer.mode = "edit"; render(); });
  document.querySelector("#policy-form")?.addEventListener("submit", savePolicy);
}

function savePolicy(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  try {
    const tableName = String(data.get("table_name")).trim();
    if (!/^[a-zA-Z0-9_]+$/.test(tableName) || tableName.startsWith("rcc_")) throw new Error("table_name 必须是安全标识符，且不能使用 rcc_ 前缀");
	const queryPolicyCode = String(data.get("query_policy_code"));
	const mutationPolicyCode = String(data.get("mutation_policy_code"));
	if (queryPolicyFor(queryPolicyCode)?.status !== "ACTIVE" || mutationPolicyFor(mutationPolicyCode)?.status !== "ACTIVE") throw new Error("新建或替换只能选择 Active 规则");
    if (state.policyDrawer.mode === "create") {
      if (policyFor(tableName)) throw new Error("该 table_name 已存在表规则");
	  policies.push({ table_name: tableName, query_policy_code: queryPolicyCode, mutation_policy_code: mutationPolicyCode, enabled: false, creator: "local-admin", modifier: "local-admin", gmt_created: nowText(), gmt_modified: nowText() });
      managedTables[tableName] = { label: tableName, columns: ["id", "value", "updated_at"], rows: [] };
	  state.policyDrawer = { open: true, mode: "view", tableName }; showToast(`${tableName} 表规则已创建，当前为停用状态`);
    } else {
      const policy = policyFor(state.policyDrawer.tableName);
	  policy.query_policy_code = queryPolicyCode; policy.mutation_policy_code = mutationPolicyCode; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); state.policyDrawer.mode = "view"; showToast(`${policy.table_name} 表规则分配已原子替换`);
    }
    state.policyError = ""; render();
  } catch (error) { state.policyError = error.message; render(); }
}

function bindPicker() {
  document.querySelector("[data-toggle-picker]")?.addEventListener("click", () => { state.tablePickerOpen = !state.tablePickerOpen; render(); if (state.tablePickerOpen) document.querySelector("#table-search")?.focus(); });
  document.querySelector("#table-search")?.addEventListener("input", (event) => { state.tableSearch = event.target.value; document.querySelectorAll("[data-select-table]").forEach((button) => { button.hidden = !button.dataset.selectTable.toLowerCase().includes(state.tableSearch.toLowerCase()); }); });
  document.querySelectorAll("[data-select-table]").forEach((button) => button.addEventListener("click", () => { state.selectedTable = button.dataset.selectTable; state.tablePickerOpen = false; state.tableSearch = ""; state.conditions = []; state.filteredRows = null; render(); }));
}

function bindContentEvents() {
  bindPicker();
  document.querySelector("[data-view-current-policy]")?.addEventListener("click", () => { state.page = "policies"; state.policyDrawer = { open: true, mode: "view", tableName: state.selectedTable }; render(); });
  document.querySelector("[data-add-condition]")?.addEventListener("click", () => { if (state.conditions.length >= 20) return showToast("最多允许 20 个查询条件", "error"); state.conditions.push({ field: managedTables[state.selectedTable].columns[0], operator: "exact", value: "" }); render(); });
  document.querySelectorAll("[data-remove-condition]").forEach((button) => button.addEventListener("click", () => { state.conditions.splice(Number(button.dataset.removeCondition), 1); render(); }));
  document.querySelectorAll("[data-condition-field]").forEach((field) => field.addEventListener("change", () => (state.conditions[Number(field.dataset.conditionField)].field = field.value)));
  document.querySelectorAll("[data-condition-operator]").forEach((field) => field.addEventListener("change", () => (state.conditions[Number(field.dataset.conditionOperator)].operator = field.value)));
  document.querySelectorAll("[data-condition-value]").forEach((field) => field.addEventListener("input", () => (state.conditions[Number(field.dataset.conditionValue)].value = field.value)));
  document.querySelector("[data-query]")?.addEventListener("click", runQuery);
  document.querySelector("[data-add-row]")?.addEventListener("click", () => { state.contentDrawer = { open: true, mode: "add", rowID: null }; state.contentError = ""; render(); });
  document.querySelectorAll("[data-edit-row]").forEach((button) => button.addEventListener("click", () => { state.contentDrawer = { open: true, mode: "edit", rowID: button.dataset.editRow }; state.contentError = ""; render(); }));
  document.querySelectorAll("[data-delete-row]").forEach((button) => button.addEventListener("click", () => { if (button.disabled) return; state.confirmDelete = button.dataset.deleteRow; render(); }));
  document.querySelectorAll("[data-close-content]").forEach((button) => button.addEventListener("click", (event) => { event.preventDefault(); state.contentDrawer.open = false; render(); }));
  document.querySelector("#content-form")?.addEventListener("submit", saveContent);
  document.querySelector("[data-cancel-delete]")?.addEventListener("click", () => { state.confirmDelete = null; render(); });
  document.querySelector("[data-confirm-delete]")?.addEventListener("click", confirmDelete);
  document.querySelectorAll(".form-switch input").forEach((input) => input.addEventListener("change", () => { input.closest(".form-switch").querySelector("strong").textContent = input.checked ? "启用" : "停用"; }));
}

function runQuery() {
  state.queryLoading = true; state.contentDrawer.open = false; render();
  setTimeout(() => {
    const rows = managedTables[state.selectedTable].rows;
    state.filteredRows = rows.filter((row) => state.conditions.every((condition) => { const actual = String(row[condition.field] ?? "").toLowerCase(); const expected = condition.value.toLowerCase(); if (!expected) return true; return condition.operator === "contains" ? actual.includes(expected) : actual === expected; }));
    state.queryLoading = false; showToast(`查询完成，共 ${state.filteredRows.length} 条记录`); render();
  }, 480);
}

function saveContent(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  const table = managedTables[state.selectedTable];
  try {
    const values = {};
    editableColumns().forEach((column) => { values[column] = column === "enabled" ? data.get(column) === "on" : String(data.get(column) ?? "").trim(); if (column !== "enabled" && !values[column]) throw new Error(`${column} 不能为空`); });
    if ("rollout_percent" in values && (Number(values.rollout_percent) < 0 || Number(values.rollout_percent) > 100)) throw new Error("rollout_percent 必须在 0 到 100 之间");
    if (state.contentDrawer.mode === "edit") { const row = table.rows.find((item) => item.id === state.contentDrawer.rowID); Object.assign(row, values, { updated_at: nowText() }); showToast("配置内容已更新"); }
    else { const nextID = String(Math.max(0, ...table.rows.map((row) => Number(row.id))) + 1); table.rows.unshift({ id: nextID, ...values, updated_at: nowText() }); showToast("配置内容已创建"); }
    state.contentDrawer.open = false; state.contentError = ""; state.filteredRows = null; render();
  } catch (error) { state.contentError = error.message; render(); }
}

function confirmDelete() {
  const table = managedTables[state.selectedTable];
  table.rows = table.rows.filter((row) => row.id !== state.confirmDelete);
  state.confirmDelete = null; state.filteredRows = null; showToast("配置内容已删除"); render();
}

function bindEvents() { bindNavigation(); state.page === "query-policies" ? bindQueryPolicyEvents() : state.page === "mutation-policies" ? bindMutationPolicyEvents() : state.page === "policies" ? bindPolicyEvents() : bindContentEvents(); }

window.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  if (state.confirmDelete) state.confirmDelete = null;
  else if (state.mobileNavOpen) state.mobileNavOpen = false;
  else if (state.policyDrawer.open) state.policyDrawer.open = false;
  else if (state.queryPolicyDrawer.open) state.queryPolicyDrawer.open = false;
  else if (state.mutationPolicyDrawer.open) state.mutationPolicyDrawer.open = false;
  else if (state.contentDrawer.open) state.contentDrawer.open = false;
  else if (state.tablePickerOpen) state.tablePickerOpen = false;
  render();
});

render();
