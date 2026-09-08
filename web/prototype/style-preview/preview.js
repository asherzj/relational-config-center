const paths = {
  database: '<ellipse cx="12" cy="5" rx="7" ry="3"/><path d="M5 5v6c0 1.7 3.1 3 7 3s7-1.3 7-3V5M5 11v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6"/>',
  'search-file': '<path d="M13 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h7M13 3v5h5l-5-5Z"/><circle cx="15" cy="14" r="3"/><path d="m17.2 16.2 3.3 3.3"/>',
  arrows: '<path d="M4 7h16l-4-4M20 17H4l4 4M4 7l4 4M20 17l-4-4"/>',
  layers: '<path d="m12 3-9 5 9 5 9-5-9-5ZM3 12l9 5 9-5M3 16l9 5 9-5"/>',
  table: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M3 10h18M9 10v10"/>',
  shield: '<path d="m12 3 8 3v5c0 5-8 10-8 10S4 16 4 11V6l8-3Z"/><path d="m8 11 3 3 5-5"/>',
  right: '<path d="m9 5 7 7-7 7"/>', plus: '<path d="M12 5v14M5 12h14"/>',
  search: '<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>',
  refresh: '<path d="M20 5v5h-5M4 19v-5h5M19 10a7 7 0 0 0-12-5L4 8M5 14a7 7 0 0 0 12 5l3-3"/>',
  close: '<path d="m6 6 12 12M18 6 6 18"/>', info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v6M12 7h.01"/>'
};
const icon = name => `<svg viewBox="0 0 24 24" aria-hidden="true">${paths[name]}</svg>`;
document.querySelectorAll('[data-icon]').forEach(el => el.innerHTML = icon(el.dataset.icon));
const esc = value => String(value).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const data = [
  {name:'标准分页查询', code:'standard_page_query_v1', field:'id', direction:'DESC', size:20, max:200, status:'active', date:'2026-09-07 10:42', user:'admin', description:'适用于多数配置表的默认分页规则。'},
  {name:'紧凑分页查询', code:'compact_page_query_v1', field:'id', direction:'DESC', size:10, max:50, status:'draft', date:'2026-09-07 09:18', user:'local-admin', description:'小页读取，适用于高频浏览的配置内容。'},
  {name:'按更新时间排序', code:'recent_updates_query_v1', field:'updated_at', direction:'DESC', size:20, max:100, status:'active', date:'2026-09-06 16:35', user:'admin', description:'优先查看最近更新的配置记录。'},
  {name:'批量浏览查询', code:'batch_page_query_v1', field:'id', direction:'ASC', size:50, max:500, status:'draft', date:'2026-09-06 14:02', user:'admin', description:'用于需要较大分页容量的配置表。'},
  {name:'旧版分页查询', code:'legacy_page_query_v1', field:'id', direction:'ASC', size:20, max:100, status:'deprecated', date:'2026-09-04 11:26', user:'local-admin', description:'仅供既有表规则继续执行。'},
];
const labels = {active:'已激活', draft:'草稿', deprecated:'已弃用'};
const search = document.querySelector('#search');
const status = document.querySelector('#status');
const rows = document.querySelector('#rows');
const dialog = document.querySelector('#detail');
let selected = null;
function render() {
  const query = search.value.trim().toLowerCase();
  const filtered = data.filter(p => `${p.name} ${p.code}`.toLowerCase().includes(query) && (status.value === 'all' || p.status === status.value));
  rows.innerHTML = filtered.map(p => `<tr class="${selected === p ? 'selected-row' : ''}"><td class="rule-identity"><strong>${esc(p.name)}</strong><code>${esc(p.code)}</code></td><td><code class="sort-code">${esc(p.field)}</code><span class="sort-direction">${esc(p.direction)}</span></td><td class="numeric">${p.size}<small>最多 ${p.max} 条</small></td><td><span class="status-badge status-${p.status}">${labels[p.status]}</span></td><td class="timestamp">${p.date.split(' ')[0]}<small>${p.date.split(' ')[1]} · ${esc(p.user)}</small></td><td><div class="row-actions"><button data-open="${esc(p.code)}">查看</button>${p.status === 'draft' ? `<button data-edit="${esc(p.code)}">编辑</button>` : ''}</div></td></tr>`).join('');
  document.querySelector('#count').textContent = `共 ${filtered.length} 个查询规则`;
  document.querySelector('#empty').hidden = filtered.length > 0;
  document.querySelector('.table-scroll').hidden = filtered.length === 0;
}
function field(label, key, value, readonly, type = 'text') { return `<label class="field"><span>${label}</span><input name="${key}" type="${type}" value="${esc(value)}" ${readonly ? 'readonly' : 'required'} ${type === 'number' ? 'min="1" max="10000"' : ''}></label>`; }
function open(p, edit = false) {
  selected = p;
  const fresh = !p;
  const value = p || {name:'',code:'',field:'id',direction:'DESC',size:20,max:200,status:'draft',description:''};
  const readonly = !(fresh || edit);
  document.querySelector('#drawer-title').textContent = fresh ? '新建查询规则草稿' : edit ? '编辑查询规则草稿' : '查询规则详情';
  document.querySelector('#drawer-description').textContent = fresh ? '设置规则名称与默认分页方式。' : value.name;
  document.querySelector('#save').hidden = readonly;
  document.querySelector('#drawer-content').innerHTML = `<div class="policy-form"><span class="status-badge status-${value.status}">${labels[value.status]}</span>${field('规则名称','name',value.name,readonly)}${field('规则编码','code',value.code,readonly || !fresh)}<label class="field"><span>说明</span><textarea name="description" rows="3" ${readonly ? 'readonly' : ''}>${esc(value.description)}</textarea></label><section class="form-panel"><h3>排序与分页</h3><div class="form-grid">${field('默认排序字段','field',value.field,readonly)}<label class="field"><span>排序方向</span><select name="direction" ${readonly ? 'disabled' : ''}><option value="DESC" ${value.direction === 'DESC' ? 'selected' : ''}>降序 DESC</option><option value="ASC" ${value.direction === 'ASC' ? 'selected' : ''}>升序 ASC</option></select></label>${field('默认每页条数','size',value.size,readonly,'number')}${field('最多每页条数','max',value.max,readonly,'number')}</div></section><div class="form-note">${icon('info')}<span>激活后，排序与分页约束将不可修改。草稿可继续编辑。</span></div>${p ? `<dl class="audit-grid"><div><dt>规则类型</dt><dd><code>page_query</code></dd></div><div><dt>最近修改人</dt><dd>${esc(p.user)}</dd></div><div><dt>最近更新时间</dt><dd>${p.date}</dd></div></dl>` : ''}</div>`;
  render();
  dialog.showModal();
}
search.addEventListener('input', render);
status.addEventListener('change', render);
rows.addEventListener('click', e => { const button = e.target.closest('[data-open], [data-edit]'); if(button) open(data.find(p => p.code === (button.dataset.open || button.dataset.edit)), Boolean(button.dataset.edit)); });
document.querySelector('#create').onclick = () => open(null, true);
document.querySelectorAll('.close-dialog').forEach(button => button.onclick = () => dialog.close());
dialog.addEventListener('click', e => { if(e.target === dialog && e.clientX < dialog.getBoundingClientRect().left) dialog.close(); });
dialog.addEventListener('close', () => { selected = null; render(); });
document.querySelector('#reset').onclick = () => { search.value = ''; status.value = 'all'; render(); };
let noticeTimer;
function notify(message) { const notice = document.querySelector('#notice'); notice.textContent = message; notice.hidden = false; clearTimeout(noticeTimer); noticeTimer = setTimeout(() => notice.hidden = true, 2400); }
document.querySelector('#refresh').onclick = () => { render(); notify('示例规则已刷新'); };
document.querySelector('#draft-form').onsubmit = e => {
  e.preventDefault();
  if(document.querySelector('#save').hidden) return;
  const form = new FormData(e.target);
  if(Number(form.get('size')) > Number(form.get('max'))) return notify('默认每页条数不能大于最多每页条数');
  if(!selected && data.some(p => p.code === form.get('code').trim())) return notify('规则编码已存在，请使用其他编码');
  const next = {name:form.get('name').trim(),code:form.get('code').trim(),field:form.get('field').trim(),direction:form.get('direction'),size:Number(form.get('size')),max:Number(form.get('max')),description:form.get('description'),status:'draft',date:'2026-09-07 12:00',user:'local-admin'};
  if(!next.name || !next.code || !next.field) return notify('请填写规则名称、编码和排序字段');
  if(selected) Object.assign(selected, next); else data.unshift(next);
  dialog.close(); render(); notify('草稿已保存在本次预览中');
};
const themes = {blue:'低饱和灰蓝',graphite:'中性石墨灰',teal:'柔和青绿'};
function theme(key) { if(!themes[key]) key = 'blue'; document.documentElement.dataset.variant = key; document.querySelector('#theme-description').textContent = themes[key]; document.querySelectorAll('[data-theme]').forEach(button => button.setAttribute('aria-pressed', String(button.dataset.theme === key))); const url = new URL(location.href); url.searchParams.set('variant', key); history.replaceState(null,'',url); }
document.querySelectorAll('[data-theme]').forEach(button => button.onclick = () => theme(button.dataset.theme));
document.addEventListener('keydown', e => { if(dialog.open || e.target.closest('input,textarea,select,[contenteditable]') || !['ArrowLeft','ArrowRight'].includes(e.key)) return; e.preventDefault(); const keys = Object.keys(themes); theme(keys[(keys.indexOf(document.documentElement.dataset.variant) + (e.key === 'ArrowRight' ? 1 : 2)) % 3]); });
theme(new URL(location.href).searchParams.get('variant'));
render();
