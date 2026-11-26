import { Link } from "react-router-dom";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";

const Dashboard = () => {
  const { user, isAdmin, logout } = useAuth();
  const { t } = useI18n();
  return (
    <div className="stack">
      <p>
        {t("account.hello", { name: user.displayName || user.name || user.email })}{" "}
        <span className="muted">
          ({t("account.notYou")}{" "}
          <button type="button" className="link-btn" onClick={logout}>
            {t("account.logout")}
          </button>
          )
        </span>
      </p>
      <p>{t("account.dashboardIntro")}</p>
      <div className="tile-grid">
        <Link className="tile" to="/my-account/orders">
          {t("account.orders")}
        </Link>
        <Link className="tile" to="/my-account/edit-address">
          {t("account.addresses")}
        </Link>
        <Link className="tile" to="/my-account/edit-account">
          {t("account.details")}
        </Link>
        <Link className="tile" to="/my-account/security">
          {t("account.security")}
        </Link>
        {isAdmin && (
          <Link className="tile tile-accent" to="/admin">
            {t("nav.admin")}
          </Link>
        )}
      </div>
    </div>
  );
};

export default Dashboard;
