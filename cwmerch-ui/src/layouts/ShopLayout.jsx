import { useEffect, useState } from "react";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { FiMenu, FiSearch, FiShoppingBag, FiUser, FiX, FiSettings, FiPhone, FiMail, FiMapPin } from "react-icons/fi";
import { FaFacebook, FaInstagram, FaXTwitter, FaTiktok, FaYoutube, FaWhatsapp, FaPinterest } from "react-icons/fa6";
import { useShop } from "../context/ShopContext";
import { useCart } from "../context/CartContext";
import { useAuth } from "../context/AuthContext";
import { useI18n } from "../i18n/I18nContext";
import LanguageSwitcher from "./LanguageSwitcher";
import ShopClosed from "../pages/shop/ShopClosed";

// Public storefront routes replaced by the "closed" page when the shop is off
// (home, catalogue, product, category, tag, cart, checkout — not order-received/payment).
export const isClosedRoute = (pathname) => /^\/(?:shop|cart|checkout)?\/?$/.test(pathname) || /^\/product(?:-category|-tag)?\//.test(pathname);

const SOCIAL = [
  ["facebook", FaFacebook, "Facebook"],
  ["instagram", FaInstagram, "Instagram"],
  ["x", FaXTwitter, "X"],
  ["tiktok", FaTiktok, "TikTok"],
  ["youtube", FaYoutube, "YouTube"],
  ["whatsapp", FaWhatsapp, "WhatsApp"],
  ["pinterest", FaPinterest, "Pinterest"],
];

const socialHref = (key, value) => {
  if (!value) return "";
  if (/^https?:\/\//i.test(value)) return value;
  if (key === "whatsapp") return `https://wa.me/${value.replace(/[^\d]/g, "")}`;
  return value;
};

export const SocialLinks = ({ social }) => {
  const links = SOCIAL.filter(([key]) => social?.[key]);
  if (links.length === 0) return null;
  return (
    <ul className="social-links">
      {links.map(([key, Icon, label]) => (
        <li key={key}>
          <a href={socialHref(key, social[key])} target="_blank" rel="noopener noreferrer" aria-label={label} title={label}>
            <Icon aria-hidden="true" />
          </a>
        </li>
      ))}
    </ul>
  );
};

// Brand shows the shop's own logo, else the cwmerch logo (mascot + wordmark).
export const Brand = ({ appearance, name, small }) => {
  const logo = appearance?.logo?.src;
  return (
    <Link to="/" className={`brand ${small ? "brand-sm" : ""}`}>
      {logo ? <img src={logo} alt={name} className="brand-logo" /> : <img src="/cwmerch-logo-small.png" alt={name || "cwmerch"} className="brand-logo brand-logo-default" width="1229" height="590" />}
    </Link>
  );
};

const ShopLayout = () => {
  const { settings, appearance, shopName, categories, shopClosed } = useShop();
  const { count } = useCart();
  const { user, isAdmin } = useAuth();
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const [search, setSearch] = useState("");
  const [menuOpen, setMenuOpen] = useState(false);

  useEffect(() => setMenuOpen(false), [location.pathname]);

  const closedHere = shopClosed && isClosedRoute(location.pathname);
  const topCategories = categories.filter((c) => !Number(c.parent) && c.slug !== "uncategorized");
  const announcement = appearance?.announcement;
  const contact = appearance?.contact || {};
  const tagline = appearance?.tagline || settings.description;

  const onSearch = (e) => {
    e.preventDefault();
    navigate(`/shop?search=${encodeURIComponent(search.trim())}`);
  };

  return (
    <div className="storefront shop-shell">
      <a href="#main" className="skip-link">
        {t("common.skipToContent")}
      </a>
      {announcement?.enabled && announcement.text && (
        <div className="announcement">
          {announcement.link ? (
            /^https?:\/\//i.test(announcement.link) ? (
              <a href={announcement.link}>{announcement.text}</a>
            ) : (
              <Link to={announcement.link}>{announcement.text}</Link>
            )
          ) : (
            <span>{announcement.text}</span>
          )}
        </div>
      )}
      <header className="site-header">
        <div className="container header-row">
          <button
            type="button"
            className="icon-btn menu-toggle"
            aria-expanded={menuOpen}
            aria-controls="site-nav"
            aria-label={menuOpen ? t("common.closeMenu") : t("common.openMenu")}
            onClick={() => setMenuOpen((o) => !o)}
          >
            {menuOpen ? <FiX /> : <FiMenu />}
          </button>
          <div className="brand-block">
            <Brand appearance={appearance} name={shopName} />
            {tagline && <span className="tagline hide-mobile">{tagline}</span>}
          </div>
          {shopClosed ? (
            <span className="spacer" />
          ) : (
          <form className="header-search" role="search" onSubmit={onSearch}>
            <label htmlFor="site-search" className="sr-only">
              {t("shop.searchProducts")}
            </label>
            <input
              id="site-search"
              type="search"
              className="input"
              placeholder={t("shop.searchProducts")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
            <button type="submit" className="icon-btn" aria-label={t("common.search")}>
              <FiSearch />
            </button>
          </form>
          )}
          <div className="header-actions">
            <LanguageSwitcher className="hide-mobile" />
            {isAdmin && (
              <Link to="/admin" className="icon-btn" aria-label={t("nav.admin")} title={t("nav.admin")}>
                <FiSettings />
              </Link>
            )}
            <Link to="/my-account" className="icon-btn" aria-label={user ? t("nav.myAccount") : t("nav.signIn")} title={user ? t("nav.myAccount") : t("nav.signIn")}>
              <FiUser />
            </Link>
            {!shopClosed && (
              <Link to="/cart" className="icon-btn cart-link" aria-label={t("nav.cartCount", { count })} title={t("nav.cart")}>
                <FiShoppingBag />
                {count > 0 && (
                  <span className="cart-badge" aria-hidden="true">
                    {count}
                  </span>
                )}
              </Link>
            )}
          </div>
        </div>
        <nav id="site-nav" className={`site-nav ${menuOpen ? "is-open" : ""}`} aria-label={t("nav.main")}>
          <div className="container nav-row">
            <NavLink to="/" end>
              {t("nav.home")}
            </NavLink>
            {!shopClosed && <NavLink to="/shop">{t("nav.shop")}</NavLink>}
            {!shopClosed &&
              topCategories.slice(0, 8).map((c) => (
                <NavLink key={c.id} to={`/product-category/${c.slug}`}>
                  {c.name}
                </NavLink>
              ))}
            {settings.contactEnabled !== false && <NavLink to="/contact">{t("nav.contact")}</NavLink>}
            <LanguageSwitcher className="show-mobile" />
          </div>
        </nav>
      </header>
      <main id="main" className="site-main">
        {closedHere ? <ShopClosed /> : <Outlet />}
      </main>
      <footer className="site-footer">
        <div className="container footer-grid">
          <div>
            <Brand appearance={appearance} name={shopName} small />
            {tagline && <p className="footer-muted">{tagline}</p>}
            <SocialLinks social={appearance?.social} />
          </div>
          <nav aria-label={t("nav.footer")} className="footer-links">
            <h2 className="footer-title">{t("footer.shop")}</h2>
            {!shopClosed && <Link to="/shop">{t("nav.shop")}</Link>}
            <Link to="/my-account">{t("nav.myAccount")}</Link>
            {!shopClosed && <Link to="/cart">{t("nav.cart")}</Link>}
            {settings.contactEnabled !== false && <Link to="/contact">{t("nav.contact")}</Link>}
          </nav>
          {(contact.phone || contact.email || contact.address) && (
            <div className="footer-contact">
              <h2 className="footer-title">{t("footer.contact")}</h2>
              {contact.phone && (
                <a href={`tel:${contact.phone.replace(/\s+/g, "")}`}>
                  <FiPhone aria-hidden="true" /> <bdi dir="ltr">{contact.phone}</bdi>
                </a>
              )}
              {contact.email && (
                <a href={`mailto:${contact.email}`}>
                  <FiMail aria-hidden="true" /> {contact.email}
                </a>
              )}
              {contact.address && (
                <span>
                  <FiMapPin aria-hidden="true" /> {contact.address}
                </span>
              )}
            </div>
          )}
        </div>
        <div className="container footer-bottom">
          {appearance?.footerText ? <p>{appearance.footerText}</p> : <p>© {new Date().getFullYear()} {shopName}</p>}
          <p className="small">
            {t("footer.poweredBy")}{" "}
            <a href="https://gitlab.cwcloud.tech/oss/cwai/cwmerch" target="_blank" rel="noopener noreferrer">
              cwmerch
            </a>
          </p>
        </div>
      </footer>
    </div>
  );
};

export default ShopLayout;
