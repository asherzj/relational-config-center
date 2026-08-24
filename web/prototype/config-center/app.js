const iconPaths = {
  grid: '<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/>',
  database: '<ellipse cx="12" cy="5" rx="8" ry="3"/><path d="M4 5v6c0 1.7 3.6 3 8 3s8-1.3 8-3V5"/><path d="M4 11v6c0 1.7 3.6 3 8 3s8-1.3 8-3v-6"/>',
  layers: '<path d="m12 2 9 5-9 5-9-5 9-5Z"/><path d="m3 12 9 5 9-5"/><path d="m3 17 9 5 9-5"/>',
  history: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l3 2"/>',
  shield: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10Z"/><path d="m9 12 2 2 4-4"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4V21a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9A1.7 1.7 0 0 0 3 14H2.8v-4H3a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.2 7 7 4.2l.1.1a1.7 1.7 0 0 0 1.9.3 1.7 1.7 0 0 0 1-1.6v-.2h4V3a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L19.8 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4H21a1.7 1.7 0 0 0-1.6 1Z"/>',
  search: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  upload: '<path d="M12 16V3m0 0L7 8m5-5 5 5"/><path d="M5 14v6h14v-6"/>',
  filter: '<path d="M4 5h16M7 12h10M10 19h4"/>',
  columns: '<path d="M4 4h16v16H4zM10 4v16M16 4v16"/>',
  more: '<circle cx="5" cy="12" r="1"/><circle cx="12" cy="12" r="1"/><circle cx="19" cy="12" r="1"/>',
  table: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 9h18M8 9v11M16 9v11"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  code: '<path d="m8 9-4 3 4 3M16 9l4 3-4 3M14 5l-4 14"/>',
  graph: '<circle cx="5" cy="6" r="2"/><circle cx="19" cy="7" r="2"/><circle cx="12" cy="18" r="2"/><path d="m7 7 10 0M6 8l5 8M18 9l-5 7"/>',
  compass: '<circle cx="12" cy="12" r="9"/><path d="m15 9-2 5-5 2 2-5 5-2Z"/>',
  branch: '<circle cx="6" cy="5" r="2"/><circle cx="18" cy="7" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 5h3a4 4 0 0 1 4 4v7M6 7v11h10"/>',
  activity: '<path d="M3 12h4l2-7 4 14 2-7h6"/>',
  spark: '<path d="m12 3 1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8L12 3Z"/><path d="m19 17 .7 2.3L22 20l-2.3.7L19 23l-.7-2.3L16 20l2.3-.7L19 17Z"/>',
  arrowLeft: '<path d="m15 18-6-6 6-6"/>',
  arrowRight: '<path d="m9 18 6-6-6-6"/>',
  x: '<path d="m6 6 12 12M18 6 6 18"/>',
  zoomIn: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4M11 8v6M8 11h6"/>',
  zoomOut: '<circle cx="11" cy="11" r="7"/><path d="m20 20-4-4M8 11h6"/>',
  fit: '<path d="M8 3H3v5M16 3h5v5M8 21H3v-5M16 21h5v-5"/>',
  alert: '<path d="M10.3 3.7 2.6 17a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.7a2 2 0 0 0-3.4 0Z"/><path d="M12 9v4M12 17h.01"/>',
};

const icon = (name, className = "") =>
  `<svg class="icon ${className}" viewBox="0 0 24 24" aria-hidden="true">${iconPaths[name] ?? iconPaths.grid}</svg>`;

const variants = [
  { key: "structured", label: "A · 结构化配置中心", hint: "Schema / 表格工作台" },
  { key: "relational", label: "B · 关系型配置中心", hint: "关系图 / 影响工作台" },
];

const structuredRows = [
  { id: "PKG-001", name: "Pro 专业版", code: "pro", price: "¥199 / 月", benefits: 12, status: "已发布", statusClass: "success", updated: "今天 10:42", owner: "林亦" },
  { id: "PKG-002", name: "Team 团队版", code: "team", price: "¥499 / 月", benefits: 18, status: "草稿", statusClass: "draft", updated: "今天 09:18", owner: "陈默" },
  { id: "PKG-003", name: "Basic 基础版", code: "basic", price: "¥59 / 月", benefits: 6, status: "已发布", statusClass: "success", updated: "昨天 18:03", owner: "林亦" },
  { id: "PKG-004", name: "Enterprise 企业版", code: "enterprise", price: "询价", benefits: 24, status: "已发布", statusClass: "success", updated: "8 月 20 日", owner: "周蕴" },
  { id: "PKG-005", name: "Starter 体验版", code: "starter", price: "免费", benefits: 4, status: "已归档", statusClass: "archived", updated: "8 月 16 日", owner: "陈默" },
];

const graphNodes = {
  price: { name: "标准定价", kind: "价格策略", symbol: "¥", color: "#8ba9ff", fields: 6, version: "v8", detail: { 环境: "生产", 计费周期: "月度", 基准价: "¥199" } },
  package: { name: "Pro 专业版", kind: "套餐", symbol: "P", color: "#4fd5b7", fields: 8, version: "v24", detail: { 状态: "已发布", 编码: "pro", 权益数: "12" } },
  benefit1: { name: "无限导出", kind: "权益", symbol: "E", color: "#b78cff", fields: 5, version: "v13", detail: { 类型: "布尔权益", 默认值: "启用", 范围: "全量数据" } },
  benefit2: { name: "AI 月度额度", kind: "权益", symbol: "E", color: "#b78cff", fields: 7, version: "v19", detail: { 类型: "计量权益", 当前额度: "10,000 次", 重置周期: "每月" } },
  channel: { name: "中国官网", kind: "销售渠道", symbol: "C", color: "#efb85e", fields: 6, version: "v11", detail: { 区域: "中国大陆", 币种: "CNY", 状态: "在售" } },
  member: { name: "专业会员", kind: "会员等级", symbol: "M", color: "#ef7d9a", fields: 9, version: "v15", detail: { 等级: "Level 3", 有效期: "订阅期内", 用户数: "8,426" } },
};

const graphPositions = {
  price: [18, 29],
  package: [48, 42],
  benefit1: [77, 18],
  benefit2: [80, 58],
  channel: [17, 73],
  member: [51, 80],
};

const graphEdges = [
  { id: "price-package", from: "price", to: "package", label: "决定价格", path: "M 180 174 C 300 174, 360 230, 480 252", labelAt: [315, 191] },
  { id: "package-benefit1", from: "package", to: "benefit1", label: "包含", path: "M 480 252 C 585 220, 650 135, 770 108", labelAt: [638, 160] },
  { id: "package-benefit2", from: "package", to: "benefit2", label: "包含", path: "M 480 252 C 610 260, 678 330, 800 348", labelAt: [650, 290] },
  { id: "channel-package", from: "channel", to: "package", label: "渠道在售", path: "M 170 438 C 255 390, 370 320, 480 252", labelAt: [298, 361], dashed: true },
  { id: "member-package", from: "member", to: "package", label: "授予套餐", path: "M 510 480 C 520 380, 500 330, 480 252", labelAt: [528, 374] },
];

const app = document.querySelector("#app");
const toast = document.querySelector("#toast");
let toastTimer;
let selectedStructured = "PKG-001";
let selectedNode = "benefit2";
let impactMode = false;

function currentVariant() {
  const value = new URLSearchParams(location.search).get("variant");
  return variants.some((variant) => variant.key === value) ? value : "structured";
}

function setVariant(next) {
  const params = new URLSearchParams(location.search);
  params.set("variant", next);
  history.replaceState({}, "", `${location.pathname}?${params}`);
  render();
}

function cycleVariant(direction) {
  const index = variants.findIndex((variant) => variant.key === currentVariant());
  const next = variants[(index + direction + variants.length) % variants.length];
  setVariant(next.key);
}

function showToast(message) {
  clearTimeout(toastTimer);
  toast.textContent = message;
  toast.classList.add("show");
  toastTimer = setTimeout(() => toast.classList.remove("show"), 2600);
}

function prototypeSwitcher() {
  const current = variants.find((variant) => variant.key === currentVariant());
  return `
    <nav class="prototype-switcher" aria-label="原型版本切换">
      <button data-switch="-1" aria-label="上一个版本">${icon("arrowLeft")}</button>
      <div class="switcher-label">${current.label}<span>${current.hint}</span></div>
      <button data-switch="1" aria-label="下一个版本">${icon("arrowRight")}</button>
    </nav>`;
}

function structuredRowMarkup(row) {
  return `
    <tr data-row="${row.id}" class="${selectedStructured === row.id ? "selected" : ""}">
      <td><div class="row-check"></div></td>
      <td><strong>${row.name}</strong><small>${row.id} · ${row.code}</small></td>
      <td>${row.price}</td>
      <td>${row.benefits} 项</td>
      <td><span class="tag ${row.statusClass}"><i class="status-dot"></i>${row.status}</span></td>
      <td>${row.updated}<small>${row.owner}</small></td>
      <td>${icon("more")}</td>
    </tr>`;
}

function structuredMarkup() {
  const selected = structuredRows.find((row) => row.id === selectedStructured) ?? structuredRows[0];
  return `
    <main class="shell structured-shell">
      <aside class="structured-sidebar">
        <div class="brand"><span class="brand-mark">S</span> Schemora</div>
        <div class="workspace-pill">
          <div>增长产品线<small>Production workspace</small></div>
          ${icon("arrowRight")}
        </div>
        <div class="side-label">配置工作台</div>
        <nav class="side-nav">
          <button class="side-link active">${icon("database")} 配置数据 <span class="count">8</span></button>
          <button class="side-link">${icon("layers")} Schema 模型</button>
          <button class="side-link">${icon("history")} 版本与发布 <span class="count">3</span></button>
          <button class="side-link">${icon("shield")} 校验规则</button>
        </nav>
        <div class="side-divider"></div>
        <div class="side-label">治理</div>
        <nav class="side-nav">
          <button class="side-link">${icon("check")} 审批中心 <span class="count">2</span></button>
          <button class="side-link">${icon("activity")} 操作审计</button>
          <button class="side-link">${icon("settings")} 工作区设置</button>
        </nav>
        <div class="sidebar-user"><span class="avatar">LY</span><div>林亦<small>配置管理员</small></div></div>
      </aside>

      <section class="structured-main">
        <header class="structured-topbar">
          <div class="breadcrumb"><span>配置数据</span><span>/</span><strong>会员套餐</strong></div>
          <div class="top-actions">
            <span class="env-badge"><i class="status-dot"></i>生产环境 · cn-prod</span>
            <button class="tool-icon" data-toast="暂无待处理通知">${icon("activity")}</button>
            <span class="avatar">LY</span>
          </div>
        </header>

        <div class="structured-content">
          <div class="page-head">
            <div>
              <p class="eyebrow">Configuration type</p>
              <h1>会员套餐</h1>
              <p>按 Schema 管理每条套餐记录，提交时独立校验并发布。</p>
            </div>
            <div class="page-head-actions">
              <button class="btn btn-secondary" data-action="import">${icon("upload")} 导入</button>
              <button class="btn btn-primary" data-action="create">${icon("plus")} 新建配置</button>
            </div>
          </div>

          <div class="metric-strip">
            <div class="metric"><span>配置记录</span><strong>24</strong><em>+3 本月</em></div>
            <div class="metric"><span>已发布</span><strong>21</strong><em>87.5%</em></div>
            <div class="metric"><span>Schema 字段</span><strong>8</strong><em>全部有效</em></div>
            <div class="metric"><span>最近发布</span><strong>v24</strong><em>10:42</em></div>
          </div>

          <div class="structured-grid">
            <section class="card table-card">
              <div class="table-tabs">
                <button class="table-tab active">数据记录</button>
                <button class="table-tab" data-toast="切换到 Schema 版本历史">版本历史</button>
                <button class="table-tab" data-toast="打开导入任务列表">导入任务</button>
              </div>
              <div class="table-tools">
                <label class="search-box">${icon("search")}<input id="record-search" placeholder="搜索名称或编码…" /></label>
                <div class="tool-buttons">
                  <button class="tool-icon" data-toast="筛选器：状态、负责人、更新时间">${icon("filter")}</button>
                  <button class="tool-icon" data-toast="自定义显示字段">${icon("columns")}</button>
                </div>
              </div>
              <table class="data-table">
                <thead><tr><th style="width: 34px"></th><th>套餐名称</th><th>基础价格</th><th>权益数量</th><th>状态</th><th>最后更新</th><th></th></tr></thead>
                <tbody id="structured-rows">${structuredRows.map(structuredRowMarkup).join("")}</tbody>
              </table>
              <div class="pagination"><span>共 24 条记录 · 每页 5 条</span><div class="page-buttons"><button class="page-button">‹</button><button class="page-button active">1</button><button class="page-button">2</button><button class="page-button">3</button><button class="page-button">›</button></div></div>
            </section>

            <aside class="card schema-card">
              <div class="schema-head">
                <div class="schema-title"><h3>Schema 检查器</h3><span class="version">SCHEMA v8</span></div>
                <div class="schema-object"><span class="object-icon">${icon("table")}</span><div><strong>${selected.name}</strong><small>${selected.id} · 当前选中记录</small></div></div>
              </div>
              <div class="schema-section">
                <h4>字段定义</h4>
                <div class="field-row"><span class="field-name"><i class="type-icon">Aa</i>name *</span><span class="field-type">string</span></div>
                <div class="field-row"><span class="field-name"><i class="type-icon">#</i>base_price *</span><span class="field-type">decimal</span></div>
                <div class="field-row"><span class="field-name"><i class="type-icon">≡</i>status *</span><span class="field-type">enum</span></div>
                <div class="field-row"><span class="field-name"><i class="type-icon">[ ]</i>benefit_codes</span><span class="field-type">string[]</span></div>
                <div class="field-row"><span class="field-name"><i class="type-icon">⌁</i>metadata</span><span class="field-type">json</span></div>
              </div>
              <div class="schema-section">
                <h4>当前记录</h4>
                <div class="field-row"><span class="field-name">基础价格</span><strong>${selected.price}</strong></div>
                <div class="field-row"><span class="field-name">权益数量</span><strong>${selected.benefits}</strong></div>
                <div class="field-row"><span class="field-name">负责人</span><strong>${selected.owner}</strong></div>
              </div>
              <div class="schema-section">
                <div class="validation-box">${icon("check")}<div><strong>通过 12 项校验</strong>字段类型、必填项、枚举值和唯一性均有效。</div></div>
              </div>
            </aside>
          </div>
        </div>
      </section>

      <dialog class="prototype-dialog" id="create-dialog">
        <form method="dialog" id="create-form">
          <div class="dialog-head"><div><h2>新建会员套餐</h2><p>表单由“会员套餐 v8”Schema 自动生成</p></div><button class="dialog-close" value="cancel" aria-label="关闭">${icon("x")}</button></div>
          <div class="dialog-body">
            <div class="form-field full"><label>套餐名称 *</label><input required value="Growth 增长版" /></div>
            <div class="form-field"><label>套餐编码 *</label><input required value="growth" /></div>
            <div class="form-field"><label>基础价格 *</label><input required value="299" /></div>
            <div class="form-field"><label>计费周期</label><select><option>按月</option><option>按年</option></select></div>
            <div class="form-field"><label>状态</label><select><option>草稿</option><option>已发布</option></select></div>
          </div>
          <div class="dialog-foot"><button class="btn btn-secondary" value="cancel">取消</button><button class="btn btn-primary" value="default">通过校验并创建</button></div>
        </form>
      </dialog>
      ${prototypeSwitcher()}
    </main>`;
}

function relatedNodes(id) {
  return graphEdges
    .filter((edge) => edge.from === id || edge.to === id)
    .map((edge) => ({ edge, id: edge.from === id ? edge.to : edge.from }));
}

function impactedNodes(id) {
  const impactMap = {
    benefit2: ["package", "member", "channel"],
    benefit1: ["package", "member", "channel"],
    price: ["package", "member", "channel"],
    package: ["benefit1", "benefit2", "member", "channel"],
    member: ["package", "benefit1", "benefit2"],
    channel: ["package"],
  };
  return impactMap[id] ?? [];
}

function graphLineMarkup(edge, connected, impacted) {
  const active = connected.includes(edge.from) && connected.includes(edge.to);
  const isImpact = impactMode && impacted.includes(edge.from) && impacted.includes(edge.to);
  return `
    <path class="graph-line ${edge.dashed ? "dashed" : ""} ${active ? "active" : ""} ${isImpact ? "impact" : ""}" d="${edge.path}" marker-end="url(#arrow-${isImpact ? "impact" : active ? "active" : "default"})" />
    <text class="line-label" x="${edge.labelAt[0]}" y="${edge.labelAt[1]}">${edge.label}</text>`;
}

function graphNodeMarkup(id, node, connected, impacted) {
  const [left, top] = graphPositions[id];
  return `
    <button class="graph-node ${selectedNode === id ? "selected" : ""} ${connected.includes(id) && selectedNode !== id ? "connected" : ""} ${impactMode && impacted.includes(id) && selectedNode !== id ? "impacted" : ""}"
      style="left:${left}%;top:${top}%;--node-color:${node.color}" data-node="${id}">
      <span class="node-top"><i class="node-symbol">${node.symbol}</i><strong>${node.name}</strong><i class="node-status"></i></span>
      <span class="node-meta"><span>${node.kind} · ${node.fields} 字段</span><span class="impact-badge">受影响</span><span>${node.version}</span></span>
    </button>`;
}

function relationInspectorMarkup() {
  const node = graphNodes[selectedNode];
  const related = relatedNodes(selectedNode);
  const impacted = impactedNodes(selectedNode);
  return `
    <aside class="inspector-panel">
      <div class="inspector-head">
        <div class="inspector-headline"><div><h2>${node.name}</h2><p>${selectedNode} · ${node.version} · 生产环境</p></div><span class="entity-kind">${node.kind}</span></div>
        <div class="inspector-tabs"><button class="inspector-tab active">关系</button><button class="inspector-tab" data-toast="切换到字段定义">字段</button><button class="inspector-tab" data-toast="切换到版本历史">版本</button></div>
      </div>
      <div class="inspector-section">
        <h3>配置摘要</h3>
        ${Object.entries(node.detail).map(([key, value]) => `<div class="relation-field"><span>${key}</span><strong>${value}</strong></div>`).join("")}
      </div>
      <div class="inspector-section">
        <h3>直接关系 · ${related.length}</h3>
        ${related.map(({ edge, id }) => `<button class="relation-row" data-node="${id}"><i class="relation-row-icon">${graphNodes[id].symbol}</i><span><strong>${graphNodes[id].name}</strong><small>${edge.label} · ${graphNodes[id].kind}</small></span><span class="arrow">›</span></button>`).join("")}
      </div>
      <div class="inspector-section">
        <h3>变更影响</h3>
        ${impactMode ? `
          <div class="impact-card">
            <div class="impact-card-head">${icon("alert")} 发现 ${impacted.length} 个下游影响</div>
            <p>修改 <strong>${node.name}</strong> 后，需要重新校验并一起发布相关配置。</p>
            <div class="impact-list">${impacted.map((id) => `<span>${graphNodes[id].name}</span>`).join("")}</div>
          </div>` : `
          <button class="btn btn-secondary" style="width:100%;background:#192330;border-color:#314052;color:#b9c5d3" data-action="impact">${icon("spark")} 模拟一次字段变更</button>`}
      </div>
    </aside>`;
}

function relationalMarkup() {
  const direct = relatedNodes(selectedNode).map((item) => item.id);
  const connected = [selectedNode, ...direct];
  const impacted = [selectedNode, ...impactedNodes(selectedNode)];
  return `
    <main class="shell relation-shell">
      <aside class="relation-rail">
        <div class="relation-logo">${icon("graph")}</div>
        <nav class="rail-nav">
          <button class="rail-button active" aria-label="关系画布">${icon("graph")}</button>
          <button class="rail-button" data-toast="打开实体目录" aria-label="实体目录">${icon("database")}</button>
          <button class="rail-button" data-toast="打开版本分支" aria-label="版本分支">${icon("branch")}</button>
          <button class="rail-button" data-toast="打开变更活动" aria-label="变更活动">${icon("activity")}</button>
        </nav>
        <div class="rail-bottom"><button class="rail-button">${icon("settings")}</button><span class="avatar">LY</span></div>
      </aside>

      <section class="relation-main">
        <header class="relation-topbar">
          <div class="relation-project">
            ${icon("graph")}
            <div><h1>会员商业化配置图</h1><span>增长产品线 / Production</span></div>
            <span class="draft-badge"><i></i>草稿分支 v25</span>
          </div>
          <div class="relation-actions">
            <button class="btn btn-secondary" data-action="validate">${icon("check")} 校验关系图</button>
            <button class="btn btn-primary" data-action="publish">${icon("upload")} 发布关联版本</button>
          </div>
        </header>

        <div class="relation-workspace">
          <aside class="entity-panel">
            <div class="panel-heading"><h2>实体目录</h2><button class="small-square" data-toast="创建新实体">${icon("plus")}</button></div>
            <label class="entity-search">${icon("search")}<input placeholder="搜索实体或关系…" /></label>
            <div class="entity-group-label">配置实体</div>
            <div class="entity-list">
              ${Object.entries(graphNodes).map(([id, node]) => `<button class="entity-item ${id === selectedNode ? "active" : ""}" data-node="${id}" style="--entity-color:${node.color}"><i class="entity-dot"></i>${node.kind}<small>${node.fields}</small></button>`).join("")}
            </div>
            <div class="entity-group-label">关系语义</div>
            <div class="relation-type"><i class="relation-type-line"></i>强依赖 / 随版本发布</div>
            <div class="relation-type"><i class="relation-type-line dashed"></i>适用范围 / 可独立发布</div>
          </aside>

          <section class="graph-section">
            <div class="graph-toolbar">
              <span class="graph-chip">${icon("compass")} 全局关系图</span>
              <button class="graph-chip ${impactMode ? "active" : ""}" data-action="impact">${icon("spark")} ${impactMode ? "关闭影响视图" : "影响分析"}</button>
            </div>
            <div class="graph-canvas">
              <svg class="graph-lines" viewBox="0 0 1000 600" preserveAspectRatio="none" aria-hidden="true">
                <defs>
                  <marker id="arrow-default" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="#3d5862" /></marker>
                  <marker id="arrow-active" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="#59dec1" /></marker>
                  <marker id="arrow-impact" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 Z" fill="#efb85e" /></marker>
                </defs>
                ${graphEdges.map((edge) => graphLineMarkup(edge, connected, impacted)).join("")}
              </svg>
              ${Object.entries(graphNodes).map(([id, node]) => graphNodeMarkup(id, node, connected, impacted)).join("")}
            </div>
            <div class="graph-legend"><span class="legend-item"><i class="legend-dot"></i>当前关系</span><span class="legend-item"><i class="legend-dot warn"></i>变更影响</span></div>
            <div class="graph-minimap"><span style="left:18%;top:27%"></span><span style="left:48%;top:42%"></span><span style="left:77%;top:20%"></span><span style="left:80%;top:58%"></span><span style="left:17%;top:70%"></span><span style="left:51%;top:78%"></span></div>
            <div class="graph-zoom"><button data-toast="缩小画布">${icon("zoomOut")}</button><button data-toast="适应画布">${icon("fit")}</button><button data-toast="放大画布">${icon("zoomIn")}</button></div>
          </section>

          ${relationInspectorMarkup()}
        </div>
      </section>
      ${prototypeSwitcher()}
    </main>`;
}

function bindStructuredEvents() {
  document.querySelectorAll("[data-row]").forEach((row) => {
    row.addEventListener("click", () => {
      selectedStructured = row.dataset.row;
      render();
    });
  });

  const search = document.querySelector("#record-search");
  search?.addEventListener("input", (event) => {
    const query = event.target.value.trim().toLowerCase();
    const matches = structuredRows.filter((row) => `${row.name} ${row.code} ${row.id}`.toLowerCase().includes(query));
    document.querySelector("#structured-rows").innerHTML = matches.length
      ? matches.map(structuredRowMarkup).join("")
      : '<tr><td colspan="7" style="text-align:center;color:#9299a4;height:120px">没有匹配的配置记录</td></tr>';
    bindStructuredRowEventsOnly();
  });

  document.querySelector('[data-action="create"]')?.addEventListener("click", () => {
    document.querySelector("#create-dialog")?.showModal();
  });
  document.querySelector('[data-action="import"]')?.addEventListener("click", () => showToast("已打开批量导入向导（原型演示）"));
  document.querySelector("#create-form")?.addEventListener("submit", () => showToast("已通过 Schema 校验并创建为草稿"));
}

function bindStructuredRowEventsOnly() {
  document.querySelectorAll("[data-row]").forEach((row) => {
    row.addEventListener("click", () => {
      selectedStructured = row.dataset.row;
      render();
    });
  });
}

function bindRelationalEvents() {
  document.querySelectorAll("[data-node]").forEach((node) => {
    node.addEventListener("click", () => {
      selectedNode = node.dataset.node;
      impactMode = false;
      render();
    });
  });
  document.querySelectorAll('[data-action="impact"]').forEach((button) => {
    button.addEventListener("click", () => {
      impactMode = !impactMode;
      render();
      if (impactMode) showToast(`已沿关系路径计算 ${impactedNodes(selectedNode).length} 个下游影响`);
    });
  });
  document.querySelector('[data-action="validate"]')?.addEventListener("click", () => showToast("关系图校验通过：无断裂引用、无循环依赖"));
  document.querySelector('[data-action="publish"]')?.addEventListener("click", () => showToast("已生成关联发布快照：6 个实体 · 5 条关系"));
}

function bindSharedEvents() {
  document.querySelectorAll("[data-switch]").forEach((button) => {
    button.addEventListener("click", () => cycleVariant(Number(button.dataset.switch)));
  });
  document.querySelectorAll("[data-toast]").forEach((button) => {
    button.addEventListener("click", () => showToast(button.dataset.toast));
  });
}

function render() {
  const variant = currentVariant();
  document.documentElement.dataset.variant = variant;
  app.innerHTML = variant === "relational" ? relationalMarkup() : structuredMarkup();
  bindSharedEvents();
  variant === "relational" ? bindRelationalEvents() : bindStructuredEvents();
}

window.addEventListener("keydown", (event) => {
  const tag = document.activeElement?.tagName;
  const editing = ["INPUT", "TEXTAREA", "SELECT"].includes(tag) || document.activeElement?.isContentEditable;
  if (editing) return;
  if (event.key === "ArrowLeft") cycleVariant(-1);
  if (event.key === "ArrowRight") cycleVariant(1);
});

window.addEventListener("popstate", render);
render();
