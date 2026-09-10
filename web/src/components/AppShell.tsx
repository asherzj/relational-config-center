import { useAccountRole, roleLabels } from "../features/accounts/roles";
import { useWorkspaceIdentity } from "../features/accounts/ProtectedWorkspace";
import { ArrowLeftRight, Bell, ChevronRight, Database, FileSearch, Layers3, Menu, ShieldCheck, Table2, X } from "lucide-react";
import { useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { Button } from "./ui/Button";
import { Badge } from "./shadcn/badge";
import { Separator } from "./shadcn/separator";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "./shadcn/dropdown-menu";

export function AppShell() {
  const identity = useWorkspaceIdentity();
  const administrator = useAccountRole("ADMIN");
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const { pathname } = useLocation();
  const section = pathname.startsWith("/configuration")
    ? "配置管理"
    : (pathname.startsWith("/platform/account-roles") || pathname.startsWith("/platform/approval-roles")) ? "平台人员管理" : "表配置管理";
  return (
    <div className="app-shell">
      <header className="app-header">
        <div className="brand">
          <Button variant="ghost" className="icon-button mobile-menu" aria-label="打开导航" aria-expanded={mobileNavOpen} aria-controls="primary-navigation" onClick={() => setMobileNavOpen(true)}>
            <Menu size={20} />
          </Button>
          <span className="brand-mark"><Database size={20} strokeWidth={1.8} /></span>
          <strong>关系型配置中心</strong>
        </div>
        <div className="header-breadcrumb"><span>工作空间</span><ChevronRight size={14} /><strong>{section}</strong></div>
        <Badge variant="outline" className="header-status"><Database size={14} />单数据源</Badge>
      </header>

      <aside id="primary-navigation" className={`sidebar ${mobileNavOpen ? "sidebar-open" : ""}`} aria-label="主导航">
        <div className="sidebar-mobile-head">
          <strong>导航</strong>
          <Button variant="ghost" className="icon-button" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)}><X size={20} /></Button>
        </div>
        <nav>
          <section className="nav-group" aria-labelledby="configuration-navigation-title">
            <div id="configuration-navigation-title" className="nav-group-title">配置管理</div>
            <NavLink to="/configuration/release-orders" onClick={() => setMobileNavOpen(false)}><FileSearch size={18} />发布单</NavLink>
            <NavLink to="/configuration/notifications" onClick={() => setMobileNavOpen(false)}><Bell size={18} />通知中心</NavLink>
            <NavLink to="/configuration/managed-data" onClick={() => setMobileNavOpen(false)}><Table2 size={18} />配置内容管理</NavLink>
          </section>
          <section className="nav-group" aria-labelledby="table-navigation-title">
            <div id="table-navigation-title" className="nav-group-title">表配置管理</div>
            <NavLink to="/platform/query-policies" onClick={() => setMobileNavOpen(false)}><FileSearch size={18} />查询规则定义</NavLink>
            <NavLink to="/platform/mutation-policies" onClick={() => setMobileNavOpen(false)}><ArrowLeftRight size={18} />变更规则定义</NavLink>
            <NavLink to="/platform/table-policies" onClick={() => setMobileNavOpen(false)}><Layers3 size={18} />表规则分配</NavLink>
          </section>
          {administrator && <section className="nav-group" aria-labelledby="people-navigation-title">
            <div id="people-navigation-title" className="nav-group-title">平台人员管理</div>
            <NavLink to="/platform/approval-roles" onClick={() => setMobileNavOpen(false)}><ShieldCheck size={18} />角色管理</NavLink>
            <NavLink to="/platform/account-roles" onClick={() => setMobileNavOpen(false)}><ShieldCheck size={18} />账号角色</NavLink>
          </section>}
        </nav>
        <div className="sidebar-account">
          <Separator />
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" className="operator w-full justify-start" aria-label="本地账号入口">
                <span className="operator-avatar"><ShieldCheck size={19} /></span>
                <span><strong>{identity?.account.display_name}</strong><small>{identity?.account.username}</small></span>
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent side="top" align="start" className="w-56">
              <DropdownMenuLabel>当前账号 · {identity?.account.roles.map(role => roleLabels[role]).join("、")}</DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild><NavLink to="/account">本地账号入口</NavLink></DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </aside>
      {mobileNavOpen && <button className="mobile-scrim" aria-label="关闭导航" onClick={() => setMobileNavOpen(false)} />}
      <div className="app-content"><Outlet /></div>
    </div>
  );
}
