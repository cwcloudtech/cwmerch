import { Link } from "react-router-dom";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { Empty } from "../../components/ui";

const NotFound = () => {
  const { t } = useI18n();
  useSeo({ title: t("notFound.title"), noindex: true });
  return (
    <div className="container">
      <h1>{t("notFound.title")}</h1>
      <Empty body={t("notFound.body")}>
        <Link to="/" className="btn btn-primary">
          {t("nav.home")}
        </Link>
      </Empty>
    </div>
  );
};

export default NotFound;
