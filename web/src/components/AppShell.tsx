import { Box, ChevronDown, ChevronRight, Database, Menu, Settings, X } from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";

export function AppShell() {
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <button className="icon-button mobile-menu" aria-label="打开导航" onClick={() => setMobileNavOpen(true)}>
            <Menu size={20} />
          </button>
          <Database className="brand-icon" size={26} />
          <strong>关系型配置中心</strong>
        </div>
        <div className="header-status">
          <span><i className="ready-dot" />MySQL · 就绪</span>
          <i className="status-divider" />
          <button className="environment">本地环境 <ChevronDown size={15} /></button>
        </div>
      </header>

      <aside className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`} aria-label="主导航">
        <div className="sidebar-mobile-head">
          <strong>导航</strong>
          <button className="icon-button" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)}><X size={20} /></button>
        </div>
        <nav>
          <section className="nav-group">
            <div className="nav-group-title"><span><Box size={18} />平台管理</span><ChevronDown size={16} /></div>
            <NavLink to="/platform/query-policies" onClick={() => setMobileNavOpen(false)}>查询策略定义</NavLink>
            <span className="nav-disabled">变更策略定义</span>
            <span className="nav-disabled">表策略分配</span>
          </section>
          <section className="nav-group nav-group-collapsed">
            <div className="nav-group-title"><span><Settings size={18} />配置管理</span><ChevronRight size={16} /></div>
            <span className="nav-disabled">配置内容管理</span>
          </section>
        </nav>
        <div className="operator">
          <span className="operator-avatar">OP</span>
          <span><strong>Admin Operator</strong><small>由服务端配置</small></span>
        </div>
      </aside>
      {mobileNavOpen && <button className="mobile-scrim" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)} />}
      <div className="app-content"><Outlet /></div>
    </div>
  );
}
