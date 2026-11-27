import { Link } from "react-router-dom";
import { useI18n } from "../i18n/I18nContext";

// Breadcrumbs: [{label, to?}] — the last crumb is the current page.
const Breadcrumbs = ({ items }) => {
  const { t } = useI18n();
  return (
    <nav className="breadcrumbs" aria-label={t("nav.breadcrumbs")}>
      <ol>
        <li>
          <Link to="/">{t("nav.home")}</Link>
        </li>
        {items.map((c, i) => (
          <li key={i}>
            {c.to && i < items.length - 1 ? <Link to={c.to}>{c.label}</Link> : <span aria-current="page">{c.label}</span>}
          </li>
        ))}
      </ol>
    </nav>
  );
};

export default Breadcrumbs;
