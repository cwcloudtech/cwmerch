import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../../context/AuthContext";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import Login from "./Login";
import AuthLogo from "../../components/AuthLogo";

const LINKS = [
  ["", "account.dashboard", true],
  ["orders", "account.orders"],
  ["downloads", "account.downloads"],
  ["edit-address", "account.addresses"],
  ["edit-account", "account.details"],
  ["security", "account.security"],
  ["api-keys", "account.apiKeys"],
  ["assistant", "account.assistant"],
];

const AccountLayout = () => {
  const { user, logout } = useAuth();
  const { shopName } = useShop();
  const { t } = useI18n();
  const navigate = useNavigate();
  useSeo({ title: `${t("nav.myAccount")} – ${shopName}`, noindex: true });

  return (
    <div className="container account-page">
      <h1>{t("nav.myAccount")}</h1>
      {!user ? (
        <>
          <AuthLogo />
          <Login />
        </>
      ) : (
        <div className="account-layout">
          <nav className="account-nav" aria-label={t("nav.myAccount")}>
            {LINKS.map(([to, key, end]) => (
              <NavLink key={to} to={to ? `/my-account/${to}` : "/my-account"} end={end}>
                {t(key)}
              </NavLink>
            ))}
            <button
              type="button"
              className="link-btn"
              onClick={async () => {
                await logout();
                navigate("/my-account");
              }}
            >
              {t("account.logout")}
            </button>
          </nav>
          <div className="account-content">
            <Outlet />
          </div>
        </div>
      )}
    </div>
  );
};

export default AccountLayout;
