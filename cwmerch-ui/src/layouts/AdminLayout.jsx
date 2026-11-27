import { useEffect, useState } from "react";
import { Link, Navigate, NavLink, Outlet, useLocation } from "react-router-dom";
import {
  FiBarChart2,
  FiBox,
  FiCpu,
  FiFile,
  FiFileText,
  FiGrid,
  FiHome,
  FiLogOut,
  FiMenu,
  FiPercent,
  FiSettings,
  FiShield,
  FiShoppingCart,
  FiSliders,
  FiUsers,
  FiX,
  FiList,
  FiDroplet,
  FiStar,
} from "react-icons/fi";
import { useAuth } from "../context/AuthContext";
import { useI18n } from "../i18n/I18nContext";
import { useSeo } from "../lib/seo";
import LanguageSwitcher from "./LanguageSwitcher";

const NAV = [
  { to: "/admin", key: "admin.nav.dashboard", icon: FiHome, end: true },
  { to: "/admin/orders", key: "admin.nav.orders", icon: FiShoppingCart },
  { to: "/admin/invoices", key: "admin.nav.invoices", icon: FiFile },
  {
    to: "/admin/products",
    key: "admin.nav.products",
    icon: FiBox,
    children: [
      { to: "/admin/products", key: "admin.nav.allProducts", end: true },
      { to: "/admin/products/new", key: "admin.nav.addProduct" },
      { to: "/admin/categories", key: "admin.nav.categories" },
      { to: "/admin/tags", key: "admin.nav.tags" },
      { to: "/admin/attributes", key: "admin.nav.attributes" },
    ],
  },
  { to: "/admin/reviews", key: "admin.nav.reviews", icon: FiStar },
  { to: "/admin/coupons", key: "admin.nav.coupons", icon: FiPercent },
  { to: "/admin/customers", key: "admin.nav.customers", icon: FiUsers },
  { to: "/admin/reports", key: "admin.nav.reports", icon: FiBarChart2 },
  { to: "/admin/accounting", key: "admin.nav.accounting", icon: FiFileText },
  { to: "/admin/appearance", key: "admin.nav.appearance", icon: FiDroplet },
  { to: "/admin/settings", key: "admin.nav.settings", icon: FiSettings },
  { to: "/admin/assistant", key: "admin.nav.assistant", icon: FiCpu },
];

const SUPER_NAV = [
  { to: "/admin/users", key: "admin.nav.users", icon: FiSliders },
  { to: "/admin/audit", key: "admin.nav.audit", icon: FiList },
  { to: "/admin/ip-bans", key: "admin.nav.ipBans", icon: FiShield },
];

const AdminLayout = () => {
  const { user, isAdmin, isSuperuser, logout } = useAuth();
  const { t } = useI18n();
  const location = useLocation();
  const [open, setOpen] = useState(false);
  useSeo({ title: `${t("nav.admin")} – cwmerch`, noindex: true });
  useEffect(() => setOpen(false), [location.pathname]);

  if (!user) return <Navigate to={`/my-account?redirect=${encodeURIComponent(location.pathname)}`} replace />;
  if (!isAdmin) return <Navigate to="/my-account" replace />;

  const items = isSuperuser ? [...NAV, ...SUPER_NAV] : NAV;
  const inSection = (item) => location.pathname.startsWith(item.to) || (item.children || []).some((c) => location.pathname.startsWith(c.to));

  return (
    <div className="admin-shell">
      <a href="#admin-main" className="skip-link">
        {t("common.skipToContent")}
      </a>
      <aside className={`admin-sidebar ${open ? "is-open" : ""}`} aria-label={t("nav.admin")}>
        <div className="admin-brand">
          <Link to="/admin" className="brand">
            <img src="/cwmerch-icon-512.png" alt="" width="36" height="36" />
            <span className="wordmark">cwmerch</span>
          </Link>
          <button type="button" className="icon-btn show-mobile" onClick={() => setOpen(false)} aria-label={t("common.closeMenu")}>
            <FiX />
          </button>
        </div>
        <nav className="admin-nav">
          {items.map((item) => {
            const Icon = item.icon;
            return (
              <div key={item.to} className={`admin-nav-group ${item.children && inSection(item) ? "is-open" : ""}`}>
                <NavLink to={item.to} end={item.end} className="admin-nav-link">
                  <Icon aria-hidden="true" /> <span>{t(item.key)}</span>
                </NavLink>
                {item.children && inSection(item) && (
                  <div className="admin-subnav">
                    {item.children.map((c) => (
                      <NavLink key={c.to} to={c.to} end={c.end}>
                        {t(c.key)}
                      </NavLink>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </nav>
        <div className="admin-sidebar-foot">
          <Link to="/" className="admin-nav-link">
            <FiGrid aria-hidden="true" /> <span>{t("admin.nav.visitStore")}</span>
          </Link>
          <button type="button" className="admin-nav-link link-like" onClick={logout}>
            <FiLogOut aria-hidden="true" className="flip-rtl" /> <span>{t("account.logout")}</span>
          </button>
        </div>
      </aside>
      {open && <div className="admin-scrim" onClick={() => setOpen(false)} aria-hidden="true" />}
      <div className="admin-body">
        <header className="admin-topbar">
          <button type="button" className="icon-btn show-mobile" onClick={() => setOpen(true)} aria-label={t("common.openMenu")} aria-expanded={open}>
            <FiMenu />
          </button>
          <span className="muted small">{user.displayName || user.email}</span>
          <span className="spacer" />
          <LanguageSwitcher />
        </header>
        <main id="admin-main" className="admin-main">
          <Outlet />
        </main>
      </div>
    </div>
  );
};

export default AdminLayout;
