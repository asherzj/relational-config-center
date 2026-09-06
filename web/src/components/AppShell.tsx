import { ArrowLeftRight, ChevronRight, Database, FileSearch, Layers3, Menu, ShieldCheck, Table2, X } from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";

export function AppShell() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const { pathname } = useLocation();
  const section = pathname.startsWith("/configuration") ? "配置管理" : "平台管理";
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <button className="icon-button mobile-menu" aria-label="打开导航" aria-expanded={mobileNavOpen} aria-controls="primary-navigation" onClick={() => setMobileNavOpen(true)}>
            <Menu size={20} />
          </button>
          <span className="brand-mark"><Database size={20} strokeWidth={1.8} /></span>
          <strong>关系型配置中心</strong>
        </div>
        <div className="header-breadcrumb"><span>工作空间</span><ChevronRight size={14} /><strong>{section}</strong></div>
        <div className="header-status">
          <Database size={14} /><span>单数据源</span>
        </div>
      </header>

      <aside id="primary-navigation" className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`} aria-label="主导航">
        <div className="sidebar-mobile-head">
          <strong>导航</strong>
          <button className="icon-button" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)}><X size={20} /></button>
        </div>
        <nav>
          <section className="nav-group">
            <div className="nav-group-title">平台管理</div>
            <NavLink to="/platform/query-policies" onClick={() => setMobileNavOpen(false)}><FileSearch size={18} />查询规则定义</NavLink>
            <NavLink to="/platform/mutation-policies" onClick={() => setMobileNavOpen(false)}><ArrowLeftRight size={18} />变更规则定义</NavLink>
            <NavLink to="/platform/table-policies" onClick={() => setMobileNavOpen(false)}><Layers3 size={18} />表规则分配</NavLink>
          </section>
          <section className="nav-group">
            <div className="nav-group-title">配置管理</div>
            <NavLink to="/configuration/managed-data" onClick={() => setMobileNavOpen(false)}><Table2 size={18} />配置内容管理</NavLink>
          </section>
        </nav>
        <div className="operator">
          <span className="operator-avatar"><ShieldCheck size={19} /></span>
          <span><strong>操作身份</strong><small>由服务端配置</small></span>
        </div>
      </aside>
      {mobileNavOpen && <button className="mobile-scrim" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)} />}
      <div className="app-content"><Outlet /></div>
    </div>
  );
}
