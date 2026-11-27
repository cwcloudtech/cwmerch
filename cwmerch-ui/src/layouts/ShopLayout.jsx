import { useCallback, useEffect, useRef, useState } from "react";
import { usePermalinks } from "../lib/permalinks";
import { Link, NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { FiChevronLeft, FiChevronRight, FiSearch, FiShoppingBag, FiUser, FiSettings, FiPhone, FiMail, FiMapPin } from "react-icons/fi";
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

// NavScroller keeps the menu on one line: it slides horizontally instead of
// wrapping, on its own (every few seconds, back to the start at the end;
// paused while hovered, focused or touched, off with reduced motion) or with
// the arrows / touch / trackpad, and brings the current page's link into view.
const AUTO_SLIDE_MS = 3500;
const NavScroller = ({ children, label }) => {
  const { t } = useI18n();
  const ref = useRef(null);
  const location = useLocation();
  const [edges, setEdges] = useState({ prev: false, next: false });
  const pausedUntil = useRef(0);
  const hovered = useRef(false);

  const update = useCallback(() => {
    const el = ref.current;
    if (!el) return;
    // RTL scrollLeft runs from 0 to negative values.
    const pos = Math.abs(el.scrollLeft);
    setEdges({ prev: pos > 2, next: pos + el.clientWidth < el.scrollWidth - 2 });
  }, []);

  useEffect(() => {
    const el = ref.current;
    if (!el) return undefined;
    update();
    el.addEventListener("scroll", update, { passive: true });
    const ro = typeof ResizeObserver !== "undefined" ? new ResizeObserver(update) : null;
    ro?.observe(el);
    window.addEventListener("resize", update);
    return () => {
      el.removeEventListener("scroll", update);
      ro?.disconnect();
      window.removeEventListener("resize", update);
    };
  }, [update, children]);

  useEffect(() => {
    const el = ref.current;
    const active = el?.querySelector("a.active");
    if (!el || !active) return;
    const box = el.getBoundingClientRect();
    const a = active.getBoundingClientRect();
    if (a.left < box.left || a.right > box.right) {
      el.scrollBy({ left: a.left - box.left - (box.width - a.width) / 2, behavior: "smooth" });
    }
  }, [location.pathname]);

  const slide = useCallback((towardEnd) => {
    const el = ref.current;
    if (!el) return;
    const rtl = getComputedStyle(el).direction === "rtl";
    const step = el.clientWidth * 0.6;
    el.scrollBy({ left: (towardEnd !== rtl ? 1 : -1) * step, behavior: "smooth" });
  }, []);

  // A manual slide pauses the automatic one for a while.
  const pause = (ms = 8000) => {
    pausedUntil.current = Date.now() + ms;
  };

  useEffect(() => {
    if (window.matchMedia?.("(prefers-reduced-motion: reduce)").matches) return undefined;
    const id = window.setInterval(() => {
      const el = ref.current;
      if (!el || hovered.current || document.hidden || Date.now() < pausedUntil.current) return;
      if (el.contains(document.activeElement)) return;
      const pos = Math.abs(el.scrollLeft);
      if (el.scrollWidth - el.clientWidth < 4) return; // everything fits
      if (pos + el.clientWidth >= el.scrollWidth - 2) el.scrollTo({ left: 0, behavior: "smooth" });
      else slide(true);
    }, AUTO_SLIDE_MS);
    return () => window.clearInterval(id);
  }, [slide]);

  return (
    <nav
      className={`site-nav ${edges.prev ? "has-prev" : ""} ${edges.next ? "has-next" : ""}`}
      aria-label={label}
      onMouseEnter={() => (hovered.current = true)}
      onMouseLeave={() => (hovered.current = false)}
      onTouchStart={() => pause()}
      onWheel={() => pause()}
    >
      <div className="container nav-wrap">
        <button type="button" className="nav-arrow nav-arrow-prev" onClick={() => (pause(), slide(false))} aria-label={t("nav.scrollPrev")} tabIndex={-1} hidden={!edges.prev}>
          <FiChevronLeft aria-hidden="true" />
        </button>
        <div className="nav-row" ref={ref}>
          {children}
        </div>
        <button type="button" className="nav-arrow nav-arrow-next" onClick={() => (pause(), slide(true))} aria-label={t("nav.scrollNext")} tabIndex={-1} hidden={!edges.next}>
          <FiChevronRight aria-hidden="true" />
        </button>
      </div>
    </nav>
  );
};

const ShopLayout = () => {
  const { settings, appearance, shopName, categories, shopClosed } = useShop();
  const { count } = useCart();
  const { user, isAdmin } = useAuth();
  const { t } = useI18n();
  const navigate = useNavigate();
  const links = usePermalinks();
  const location = useLocation();
  const [search, setSearch] = useState("");

  const closedHere = shopClosed && isClosedRoute(location.pathname);
  const topCategories = categories.filter((c) => !Number(c.parent) && c.slug !== "uncategorized");
  const announcement = appearance?.announcement;
  const contact = appearance?.contact || {};
  const tagline = appearance?.tagline || settings.description;

  const onSearch = (e) => {
    e.preventDefault();
    navigate(`${links.shop}?search=${encodeURIComponent(search.trim())}`);
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
            <LanguageSwitcher />
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
        <NavScroller label={t("nav.main")}>
            <NavLink to="/" end>
              {t("nav.home")}
            </NavLink>
            {!shopClosed && <NavLink to={links.shop}>{t("nav.shop")}</NavLink>}
            {!shopClosed &&
              topCategories.map((c) => (
                <NavLink key={c.id} to={links.category(c)}>
                  {c.name}
                </NavLink>
              ))}
            {settings.contactEnabled !== false && <NavLink to="/contact">{t("nav.contact")}</NavLink>}
        </NavScroller>
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
            {!shopClosed && <Link to={links.shop}>{t("nav.shop")}</Link>}
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
