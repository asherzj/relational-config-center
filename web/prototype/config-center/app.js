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

const queryConfig = {
  default_order: { field: "id", direction: "DESC" },
  default_page_size: 20,
  max_page_size: 200,
};

const mutationConfig = () => ({
  auto_fill: {
    add: {
      creator: { source: "operator" },
      modifier: { source: "operator" },
      gmt_created: { source: "now" },
      gmt_modified: { source: "now" },
    },
    modify: {
      modifier: { source: "operator" },
      gmt_modified: { source: "now" },
    },
  },
});

let policies = [
  {
    table_name: "payment_channels",
    query_policy: "mysql_page_query_v1",
    query_policy_config: structuredClone(queryConfig),
    mutation_policy: "mysql_single_table_mutation_v1",
    mutation_policy_config: mutationConfig(),
    allow_add: true,
    allow_modify: true,
    allow_delete: true,
    enabled: true,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-20 10:15:30",
    gmt_modified: "2026-08-24 11:42:18",
  },
  {
    table_name: "feature_flags",
    query_policy: "mysql_page_query_v1",
    query_policy_config: structuredClone(queryConfig),
    mutation_policy: "mysql_single_table_mutation_v1",
    mutation_policy_config: mutationConfig(),
    allow_add: true,
    allow_modify: true,
    allow_delete: false,
    enabled: true,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-21 09:33:21",
    gmt_modified: "2026-08-24 11:58:07",
  },
  {
    table_name: "regional_limits",
    query_policy: "mysql_page_query_v1",
    query_policy_config: structuredClone(queryConfig),
    mutation_policy: "mysql_single_table_mutation_v1",
    mutation_policy_config: mutationConfig(),
    allow_add: true,
    allow_modify: true,
    allow_delete: false,
    enabled: false,
    creator: "admin",
    modifier: "admin",
    gmt_created: "2026-08-19 16:05:11",
    gmt_modified: "2026-08-20 08:22:14",
  },
];

const emptyPolicyFilters = () => ({
  tableName: "",
  queryPolicy: "",
  queryConfig: "",
  mutationPolicy: "",
  mutationConfig: "",
  allowAdd: "",
  allowModify: "",
  allowDelete: "",
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
    columns: ["id", "code", "enabled", "rollout_percent", "gmt_modified"],
    rows: [
      { id: "42", code: "checkout_v2", enabled: true, rollout_percent: "25", gmt_modified: "2026-08-24 11:58:07" },
      { id: "43", code: "search_boost", enabled: true, rollout_percent: "80", gmt_modified: "2026-08-24 11:42:18" },
      { id: "44", code: "dark_header", enabled: false, rollout_percent: "0", gmt_modified: "2026-08-23 16:05:11" },
    ],
  },
  payment_channels: {
    label: "支付渠道",
    columns: ["id", "channel_code", "provider", "status", "gmt_modified"],
    rows: [
      { id: "101", channel_code: "bank_card", provider: "UnionPay", status: "ACTIVE", gmt_modified: "2026-08-24 10:22:31" },
      { id: "102", channel_code: "digital_wallet", provider: "WalletHub", status: "ACTIVE", gmt_modified: "2026-08-23 18:07:45" },
      { id: "103", channel_code: "bank_transfer", provider: "FastBank", status: "PAUSED", gmt_modified: "2026-08-22 09:40:16" },
    ],
  },
  regional_limits: {
    label: "区域限额",
    columns: ["id", "region", "daily_limit", "enabled", "gmt_modified"],
    rows: [],
  },
};

const state = {
  page: "policies",
  mobileNavOpen: false,
  policyDrawer: { open: false, mode: "view", tableName: "feature_flags" },
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

function navigationMarkup() {
  const platformActive = state.page === "policies";
  const configActive = state.page === "content";
  return `
    <aside class="sidebar ${state.mobileNavOpen ? "open" : ""}" aria-label="主导航">
      <div class="mobile-brand"><span class="brand-mark">${icon("database")}</span><strong>关系型配置中心</strong><button class="icon-button mobile-nav-close" data-close-nav aria-label="关闭导航">${icon("close")}</button></div>
      <nav class="nav-groups">
        <section class="nav-group ${platformActive ? "expanded" : ""}">
          <button class="nav-primary" data-nav="policies" aria-expanded="${platformActive}"><span>${icon("layers")}平台管理</span>${icon("chevronDown", "nav-chevron")}</button>
          <button class="nav-secondary ${platformActive ? "active" : ""}" data-page="policies">配置策略管理</button>
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
  if (state.policyQueryLoading) return `<tr><td colspan="14"><div class="loading-state policy-loading"><i></i><span>正在查询配置策略…</span></div></td></tr>`;
  if (!visiblePolicies.length) return `<tr><td colspan="14"><div class="empty-state policy-empty">${icon("search")}<strong>没有匹配的配置策略</strong><span>调整查询条件后重试</span></div></td></tr>`;
  return visiblePolicies.map((policy) => {
    const selected = state.policyDrawer.open && state.policyDrawer.tableName === policy.table_name;
    return `
      <tr class="${selected ? "selected" : ""}" data-policy-row="${policy.table_name}">
        <td><code>${policy.table_name}</code></td><td><code>${policy.query_policy}</code></td>
        <td><button class="config-preview" data-policy-action="view" data-table="${policy.table_name}">{…}</button></td>
        <td><code>${policy.mutation_policy}</code></td>
        <td><button class="config-preview" data-policy-action="view" data-table="${policy.table_name}">{…}</button></td>
        <td>${capabilityMarkup(policy.allow_add)}</td><td>${capabilityMarkup(policy.allow_modify)}</td><td>${capabilityMarkup(policy.allow_delete)}</td>
        <td>${statusMarkup(policy.enabled)}</td><td>${policy.creator}</td><td>${policy.modifier}</td>
        <td class="date-cell">${policy.gmt_created}</td><td class="date-cell">${policy.gmt_modified}</td>
        <td class="action-cell"><button class="link-button" data-policy-action="view" data-table="${policy.table_name}">查看</button><button class="link-button" data-policy-action="replace" data-table="${policy.table_name}">替换</button><button class="link-button ${policy.enabled ? "danger" : ""}" data-policy-action="toggle" data-table="${policy.table_name}">${policy.enabled ? "停用" : "启用"}</button></td>
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
    && contains(policy.query_policy, filters.queryPolicy)
    && contains(JSON.stringify(policy.query_policy_config), filters.queryConfig)
    && contains(policy.mutation_policy, filters.mutationPolicy)
    && contains(JSON.stringify(policy.mutation_policy_config), filters.mutationConfig)
    && (!filters.allowAdd || String(policy.allow_add) === filters.allowAdd)
    && (!filters.allowModify || String(policy.allow_modify) === filters.allowModify)
    && (!filters.allowDelete || String(policy.allow_delete) === filters.allowDelete)
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
    <form class="policy-filter-surface" id="policy-filter-form" aria-label="配置策略查询条件">
      <div class="policy-filter-grid">
        <label class="policy-filter-field"><span>表名</span><input data-policy-filter name="tableName" class="mono" value="${escapeHTML(draft.tableName)}" placeholder="请输入 table_name"></label>
        <label class="policy-filter-field range-field"><span>创建时间</span><span class="date-range"><input data-policy-filter name="createdFrom" type="date" value="${escapeHTML(draft.createdFrom)}" aria-label="创建开始日期"><i>—</i><input data-policy-filter name="createdTo" type="date" value="${escapeHTML(draft.createdTo)}" aria-label="创建结束日期"></span></label>
        <label class="policy-filter-field range-field"><span>修改时间</span><span class="date-range"><input data-policy-filter name="modifiedFrom" type="date" value="${escapeHTML(draft.modifiedFrom)}" aria-label="修改开始日期"><i>—</i><input data-policy-filter name="modifiedTo" type="date" value="${escapeHTML(draft.modifiedTo)}" aria-label="修改结束日期"></span></label>
        <label class="policy-filter-field"><span>创建人</span><select data-policy-filter name="creator"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.creator), draft.creator)}</select></label>
        <label class="policy-filter-field"><span>修改人</span><select data-policy-filter name="modifier"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.modifier), draft.modifier)}</select></label>
        <label class="policy-filter-field"><span>启用状态</span><select data-policy-filter name="enabled"><option value="">全部</option><option value="true" ${draft.enabled === "true" ? "selected" : ""}>启用</option><option value="false" ${draft.enabled === "false" ? "selected" : ""}>停用</option></select></label>
        <label class="policy-filter-field"><span>查询策略</span><select data-policy-filter name="queryPolicy"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.query_policy), draft.queryPolicy)}</select></label>
        <label class="policy-filter-field"><span>变更策略</span><select data-policy-filter name="mutationPolicy"><option value="">全部</option>${policySelectOptions(policies.map((policy) => policy.mutation_policy), draft.mutationPolicy)}</select></label>
        <label class="policy-filter-field"><span>允许新增</span><select data-policy-filter name="allowAdd"><option value="">全部</option><option value="true" ${draft.allowAdd === "true" ? "selected" : ""}>允许</option><option value="false" ${draft.allowAdd === "false" ? "selected" : ""}>禁止</option></select></label>
        <label class="policy-filter-field filter-textarea-field"><span>查询配置</span><textarea data-policy-filter name="queryConfig" class="mono" placeholder="请输入查询策略配置片段">${escapeHTML(draft.queryConfig)}</textarea></label>
        <label class="policy-filter-field filter-textarea-field"><span>变更配置</span><textarea data-policy-filter name="mutationConfig" class="mono" placeholder="请输入变更策略配置片段">${escapeHTML(draft.mutationConfig)}</textarea></label>
        <label class="policy-filter-field"><span>允许修改</span><select data-policy-filter name="allowModify"><option value="">全部</option><option value="true" ${draft.allowModify === "true" ? "selected" : ""}>允许</option><option value="false" ${draft.allowModify === "false" ? "selected" : ""}>禁止</option></select></label>
        <label class="policy-filter-field"><span>允许删除</span><select data-policy-filter name="allowDelete"><option value="">全部</option><option value="true" ${draft.allowDelete === "true" ? "selected" : ""}>允许</option><option value="false" ${draft.allowDelete === "false" ? "selected" : ""}>禁止</option></select></label>
      </div>
      <footer class="policy-filter-actions"><button class="button primary" type="submit" ${state.policyQueryLoading ? "disabled" : ""}>${icon("search")}${state.policyQueryLoading ? "查询中…" : "查询"}</button><button class="button secondary" type="button" data-reset-policy-filters>${icon("refresh")}重置</button></footer>
    </form>`;
}

function policyPageMarkup() {
  const visiblePolicies = filteredPolicies();
  return `
    <main class="workspace ${state.policyDrawer.open ? "with-drawer" : ""}">
      <div class="page-head"><div><h1>配置策略管理</h1><p>管理现有数据库表的 Query Policy 与 Mutation Policy。</p></div></div>
      ${policyFilterMarkup()}
      <div class="policy-list-toolbar"><button class="button primary" data-new-policy>${icon("plus")}新建策略</button><button class="button secondary" data-refresh>${icon("refresh")}刷新</button></div>
      <section class="table-surface" aria-label="Table Policy Catalog">
        <div class="table-scroll"><table class="data-table policy-table"><thead><tr><th>table_name</th><th>query_policy</th><th>query_policy_config</th><th>mutation_policy</th><th>mutation_policy_config</th><th>allow_add</th><th>allow_modify</th><th>allow_delete</th><th>enabled</th><th>creator</th><th>modifier</th><th>gmt_created</th><th>gmt_modified</th><th>操作</th></tr></thead><tbody>${policyRowsMarkup(visiblePolicies)}</tbody></table></div>
        <footer class="table-footer"><span>共 ${visiblePolicies.length} 条策略${visiblePolicies.length !== policies.length ? ` · 总计 ${policies.length} 条` : ""}</span><span>Policy Catalog · 实时读取</span></footer>
      </section>
    </main>${policyDrawerMarkup()}`;
}

function lifecycleMarkup(policy) {
  const steps = [["已创建", policy?.gmt_created || "保存后生成"], ["已验证", policy ? policy.gmt_created : "等待校验"], [policy?.enabled ? "已启用" : "待启用", policy?.enabled ? policy.gmt_modified : "—"]];
  return `<ol class="lifecycle">${steps.map(([label, time], index) => `<li class="${index < 2 || policy?.enabled ? "done" : ""}"><i>${icon(index < 2 || policy?.enabled ? "check" : "info")}</i><span><strong>${label}</strong><small>${time}</small></span></li>`).join("")}</ol>`;
}

function policyDrawerMarkup() {
  if (!state.policyDrawer.open) return "";
  const policy = state.policyDrawer.mode === "create" ? null : policyFor(state.policyDrawer.tableName);
  const editing = state.policyDrawer.mode !== "view";
  const title = state.policyDrawer.mode === "create" ? "新建配置策略" : state.policyDrawer.mode === "replace" ? "替换配置策略" : "配置策略详情";
  const queryJSON = JSON.stringify(policy?.query_policy_config || queryConfig, null, 2);
  const mutationJSON = JSON.stringify(policy?.mutation_policy_config || mutationConfig(), null, 2);
  return `
    <aside class="detail-drawer" role="dialog" aria-label="${title}">
      <header class="drawer-head"><h2>${title}</h2><button class="icon-button" data-close-policy aria-label="关闭">${icon("close")}</button></header>
      <form id="policy-form" class="drawer-body"><div class="policy-form-grid"><div class="policy-fields">
        <label class="form-field"><span>table_name</span><input name="table_name" class="mono" value="${escapeHTML(policy?.table_name || "")}" ${policy ? "readonly" : ""} ${editing ? "" : "disabled"} placeholder="notification_templates" required></label>
        <label class="form-field"><span>query_policy</span><select name="query_policy" class="mono" ${editing ? "" : "disabled"}><option>mysql_page_query_v1</option></select></label>
        <label class="form-field"><span>query_policy_config</span><textarea name="query_policy_config" class="json-editor" spellcheck="false" ${editing ? "" : "disabled"}>${escapeHTML(queryJSON)}</textarea></label>
        <label class="form-field"><span>mutation_policy</span><select name="mutation_policy" class="mono" ${editing ? "" : "disabled"}><option>mysql_single_table_mutation_v1</option></select></label>
        <div class="capability-grid" aria-label="Mutation 操作权限">
          <label class="switch-field"><span>allow_add</span><label class="switch"><input type="checkbox" name="allow_add" ${policy?.allow_add ? "checked" : ""} ${editing ? "" : "disabled"}><i></i></label><strong>${policy?.allow_add ? "允许" : "禁止"}</strong></label>
          <label class="switch-field"><span>allow_modify</span><label class="switch"><input type="checkbox" name="allow_modify" ${policy?.allow_modify ? "checked" : ""} ${editing ? "" : "disabled"}><i></i></label><strong>${policy?.allow_modify ? "允许" : "禁止"}</strong></label>
          <label class="switch-field danger-capability"><span>allow_delete</span><label class="switch"><input type="checkbox" name="allow_delete" ${policy?.allow_delete ? "checked" : ""} ${editing ? "" : "disabled"}><i></i></label><strong>${policy?.allow_delete ? "允许" : "禁止"}</strong></label>
        </div>
        <label class="form-field"><span>mutation_policy_config</span><textarea name="mutation_policy_config" class="json-editor tall" spellcheck="false" ${editing ? "" : "disabled"}>${escapeHTML(mutationJSON)}</textarea></label>
        <div class="switch-field"><span>enabled</span><label class="switch"><input type="checkbox" name="enabled" ${policy?.enabled ? "checked" : ""} disabled><i></i></label><strong>${policy?.enabled ? "启用" : "停用"}</strong></div>
        ${policy ? `<div class="audit-grid"><label class="form-field"><span>creator</span><input value="${policy.creator}" disabled></label><label class="form-field"><span>gmt_created</span><input value="${policy.gmt_created}" disabled></label><label class="form-field"><span>modifier</span><input value="${policy.modifier}" disabled></label><label class="form-field"><span>gmt_modified</span><input value="${policy.gmt_modified}" disabled></label></div>` : ""}
        ${state.policyError ? `<p class="form-error">${icon("info")}${escapeHTML(state.policyError)}</p>` : ""}
      </div>${lifecycleMarkup(policy)}</div></form>
      <footer class="drawer-foot">${editing ? `<button class="button primary" type="submit" form="policy-form">${state.policyDrawer.mode === "create" ? "创建策略" : "保存替换"}</button><button class="button secondary" data-close-policy>取消</button>` : `<button class="button primary" data-edit-policy>替换策略</button><button class="button secondary" data-close-policy>关闭</button>`}</footer>
    </aside>`;
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
  if (state.queryLoading) return `<tr><td colspan="${table.columns.length + 1}"><div class="loading-state"><i></i><span>正在执行 Query Policy…</span></div></td></tr>`;
  if (!rows.length) return `<tr><td colspan="${table.columns.length + 1}"><div class="empty-state">${icon("search")}<strong>没有匹配的配置内容</strong><span>调整查询条件后重试</span></div></td></tr>`;
  return rows.map((row) => `<tr>${table.columns.map((column) => `<td>${cellMarkup(column, row[column])}</td>`).join("")}<td class="action-cell"><button class="link-button" data-edit-row="${row.id}" ${policy.allow_modify ? "" : 'disabled title="Table Policy 禁止修改"'}>编辑</button><button class="link-button danger" data-delete-row="${row.id}" ${policy.allow_delete ? "" : 'disabled title="Table Policy 禁止删除"'}>删除</button></td></tr>`).join("");
}

function contentPageMarkup() {
  const table = managedTables[state.selectedTable];
  const policy = policyFor(state.selectedTable);
  return `
    <main class="workspace content-workspace ${state.contentDrawer.open ? "with-drawer" : ""}">
      <div class="page-head compact"><div><h1>配置内容管理</h1><p>选择 enabled Managed Table，并按当前 Policy Snapshot 查询或变更内容。</p></div></div>
      <section class="content-controls">${tablePickerMarkup()}<div class="policy-summary"><i class="ready-dot"></i><span>Policy <strong>已启用</strong></span><b>·</b><code>${policy.query_policy}</code><button data-view-current-policy>查看策略</button></div>
        <div class="query-builder"><div class="query-head"><strong>${icon("filter")}查询条件</strong><span>最多 20 个条件，使用 AND 连接</span></div><div class="condition-list">${conditionsMarkup(table)}</div><div class="query-actions"><button class="button dashed" data-add-condition>${icon("plus")}添加条件</button><label><span>排序</span><select id="order-field">${table.columns.map((column) => `<option ${column === "id" ? "selected" : ""}>${column}</option>`).join("")}</select></label><label><span>方向</span><select id="order-direction"><option>DESC</option><option>ASC</option></select></label><button class="button primary" data-query>${icon("search")}查询</button></div></div>
      </section>
      <section class="table-surface content-table-surface"><div class="table-toolbar"><div><strong>${table.label}</strong><code>${state.selectedTable}</code></div><button class="button primary" data-add-row ${policy.allow_add ? "" : "disabled"}>${icon("plus")}新增内容</button></div><div class="table-scroll"><table class="data-table content-table"><thead><tr>${table.columns.map((column) => `<th>${column}</th>`).join("")}<th>操作</th></tr></thead><tbody>${contentRowsMarkup(table, policy)}</tbody></table></div><footer class="table-footer"><span>共 ${visibleRows().length} 条记录</span><span>实时 Schema · JSON String</span></footer></section>
    </main>${contentDrawerMarkup()}${confirmMarkup()}`;
}

function editableColumns() {
  return managedTables[state.selectedTable].columns.filter((column) => !["id", "gmt_modified"].includes(column));
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
  return `<aside class="detail-drawer content-drawer" role="dialog" aria-label="${title}"><header class="drawer-head"><div><h2>${title}</h2><p>字段来自当前 Managed Table 实时 Schema</p></div><button class="icon-button" data-close-content aria-label="关闭">${icon("close")}</button></header><form id="content-form" class="drawer-body content-form">${editableColumns().map((column) => inputForColumn(column, row?.[column])).join("")}${state.contentError ? `<p class="form-error">${icon("info")}${escapeHTML(state.contentError)}</p>` : ""}<div class="form-note">${icon("info")}服务端 Auto Fill 将覆盖 modifier 与 gmt_modified。</div></form><footer class="drawer-foot"><button class="button primary" type="submit" form="content-form">${state.contentDrawer.mode === "edit" ? "保存修改" : "创建内容"}</button><button class="button secondary" data-close-content>取消</button></footer></aside>`;
}

function confirmMarkup() {
  if (!state.confirmDelete) return "";
  return `<div class="modal-backdrop"><section class="confirm-dialog" role="alertdialog" aria-modal="true"><i class="confirm-icon">${icon("trash")}</i><h2>删除配置内容？</h2><p>将永久删除 <code>${state.selectedTable} #${state.confirmDelete}</code>，此操作无法撤销。</p><div><button class="button danger-button" data-confirm-delete>确认删除</button><button class="button secondary" data-cancel-delete>取消</button></div></section></div>`;
}

function render() {
  app.innerHTML = `${headerMarkup()}${navigationMarkup()}${state.page === "policies" ? policyPageMarkup() : contentPageMarkup()}`;
  bindEvents();
}

function changePage(page) {
  state.page = page;
  state.mobileNavOpen = false;
  state.policyDrawer.open = false;
  state.contentDrawer.open = false;
  state.tablePickerOpen = false;
  render();
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
      showToast(`查询完成，共 ${count} 条策略`);
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
  document.querySelector("[data-refresh]")?.addEventListener("click", (event) => { event.currentTarget.classList.add("spinning"); setTimeout(() => showToast("Policy Catalog 已刷新"), 350); });
  document.querySelectorAll("[data-policy-row]").forEach((row) => row.addEventListener("click", (event) => { if (event.target.closest("button")) return; state.policyDrawer = { open: true, mode: "view", tableName: row.dataset.policyRow }; render(); }));
  document.querySelectorAll("[data-policy-action]").forEach((button) => button.addEventListener("click", () => {
    const policy = policyFor(button.dataset.table);
    const action = button.dataset.policyAction;
    if (action === "toggle") { policy.enabled = !policy.enabled; policy.modifier = "local-admin"; policy.gmt_modified = nowText(); showToast(`${policy.table_name} 已${policy.enabled ? "启用" : "停用"}`); render(); return; }
    state.policyDrawer = { open: true, mode: action, tableName: policy.table_name }; state.policyError = ""; render();
  }));
  document.querySelectorAll("[data-close-policy]").forEach((button) => button.addEventListener("click", (event) => { event.preventDefault(); state.policyDrawer.open = false; render(); }));
  document.querySelector("[data-edit-policy]")?.addEventListener("click", () => { state.policyDrawer.mode = "replace"; render(); });
  document.querySelector("#policy-form")?.addEventListener("submit", savePolicy);
}

function savePolicy(event) {
  event.preventDefault();
  const data = new FormData(event.currentTarget);
  try {
    const tableName = String(data.get("table_name")).trim();
    if (!/^[a-zA-Z0-9_]+$/.test(tableName) || tableName.startsWith("rcc_")) throw new Error("table_name 必须是安全标识符，且不能使用 rcc_ 前缀");
    const queryPolicyConfig = JSON.parse(String(data.get("query_policy_config")));
    const mutationPolicyConfig = JSON.parse(String(data.get("mutation_policy_config")));
    if (typeof queryPolicyConfig !== "object" || Array.isArray(queryPolicyConfig) || typeof mutationPolicyConfig !== "object" || Array.isArray(mutationPolicyConfig)) throw new Error("Policy Config 必须是 JSON Object");
    if (state.policyDrawer.mode === "create") {
      if (policyFor(tableName)) throw new Error("该 table_name 已存在 Table Policy");
      policies.push({ table_name: tableName, query_policy: String(data.get("query_policy")), query_policy_config: queryPolicyConfig, mutation_policy: String(data.get("mutation_policy")), mutation_policy_config: mutationPolicyConfig, allow_add: data.has("allow_add"), allow_modify: data.has("allow_modify"), allow_delete: data.has("allow_delete"), enabled: false, creator: "local-admin", modifier: "local-admin", gmt_created: nowText(), gmt_modified: nowText() });
      managedTables[tableName] = { label: tableName, columns: ["id", "value", "gmt_modified"], rows: [] };
      state.policyDrawer = { open: true, mode: "view", tableName }; showToast(`${tableName} 策略已创建，当前为停用状态`);
    } else {
      const policy = policyFor(state.policyDrawer.tableName);
      policy.query_policy_config = queryPolicyConfig; policy.mutation_policy_config = mutationPolicyConfig; policy.allow_add = data.has("allow_add"); policy.allow_modify = data.has("allow_modify"); policy.allow_delete = data.has("allow_delete"); policy.modifier = "local-admin"; policy.gmt_modified = nowText(); state.policyDrawer.mode = "view"; showToast(`${policy.table_name} 策略已完整替换`);
    }
    state.policyError = ""; render();
  } catch (error) { state.policyError = error.message.includes("JSON") ? `配置 JSON 无效：${error.message}` : error.message; render(); }
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
    if (state.contentDrawer.mode === "edit") { const row = table.rows.find((item) => item.id === state.contentDrawer.rowID); Object.assign(row, values, { gmt_modified: nowText() }); showToast("配置内容已更新"); }
    else { const nextID = String(Math.max(0, ...table.rows.map((row) => Number(row.id))) + 1); table.rows.unshift({ id: nextID, ...values, gmt_modified: nowText() }); showToast("配置内容已创建"); }
    state.contentDrawer.open = false; state.contentError = ""; state.filteredRows = null; render();
  } catch (error) { state.contentError = error.message; render(); }
}

function confirmDelete() {
  const table = managedTables[state.selectedTable];
  table.rows = table.rows.filter((row) => row.id !== state.confirmDelete);
  state.confirmDelete = null; state.filteredRows = null; showToast("配置内容已删除"); render();
}

function bindEvents() { bindNavigation(); state.page === "policies" ? bindPolicyEvents() : bindContentEvents(); }

window.addEventListener("keydown", (event) => {
  if (event.key !== "Escape") return;
  if (state.confirmDelete) state.confirmDelete = null;
  else if (state.mobileNavOpen) state.mobileNavOpen = false;
  else if (state.policyDrawer.open) state.policyDrawer.open = false;
  else if (state.contentDrawer.open) state.contentDrawer.open = false;
  else if (state.tablePickerOpen) state.tablePickerOpen = false;
  render();
});

render();
