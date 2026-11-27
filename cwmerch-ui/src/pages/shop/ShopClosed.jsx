import { Link } from "react-router-dom";
import { FiLock, FiSettings } from "react-icons/fi";
import { useShop } from "../../context/ShopContext";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";

// ShopClosed replaces the public storefront when the shop switch is off
// (GET /v1/shop/settings shopEnabled:false, or a 403 errors.shopDisabled).
const ShopClosed = () => {
  const { appearance, shopName, settings } = useShop();
  const { user, isAdmin } = useAuth();
  const { t } = useI18n();
  useSeo({ title: `${shopName} – ${t("shopClosed.title")}`, noindex: true });
  const logo = appearance?.logo?.src;
  const message = settings.closedMessage || t("shopClosed.defaultMessage");
  return (
    <div className="container shop-closed">
      <div className="card shop-closed-card stack">
        {logo ? <img src={logo} alt={shopName} className="shop-closed-logo" /> : <img src="/cwmerch-logo-small.png" alt="cwmerch" className="shop-closed-logo" width="1229" height="590" />}
        {logo || shopName !== "cwmerch" ? <h1>{shopName}</h1> : null}
        <p className="shop-closed-badge">
          <FiLock aria-hidden="true" /> {t("shopClosed.title")}
        </p>
        <p className="pre-line">{message}</p>
        <div className="row center wrap">
          {!user && (
            <Link to="/my-account" className="btn btn-primary">
              {t("nav.signIn")}
            </Link>
          )}
          {user && !isAdmin && (
            <Link to="/my-account" className="btn btn-secondary">
              {t("nav.myAccount")}
            </Link>
          )}
          {isAdmin && (
            <Link to="/admin" className="btn btn-primary">
              <FiSettings aria-hidden="true" /> {t("shopClosed.goToAdmin")}
            </Link>
          )}
        </div>
      </div>
    </div>
  );
};

export default ShopClosed;
