import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { toast } from "react-toastify";
import { FiTrendingUp, FiX, FiEdit2 } from "react-icons/fi";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { ErrorBox, Spinner, Tabs } from "../ui";
import { AiSuggestions, AnalysisList, ScoreBadges, ScoreDot, SnippetPreview } from "./SeoPanel";

const META_FIELDS = ["focusKeyphrase", "title", "description", "ogTitle", "ogDescription", "breadcrumbTitle"];

const endpointOf = (page) =>
  page.kind === "product" ? `/v1/admin/seo/products/${page.id}` : `/v1/admin/seo/terms/${page.taxonomy}/${page.id}`;

// SeoPageAudit is the storefront's "Analyse this page" button, for the shop's
// managers: the page's SEO and readability analysis, its Google preview and
// the AI's suggestions (meta fields applied at once).
const SeoPageAudit = () => {
  const { t, locale, errMsg } = useI18n();
  const location = useLocation();
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState(null);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(false);
  const [tab, setTab] = useState("seo");

  const load = async () => {
    setLoading(true);
    setError(null);
    try {
      const { data } = await api.post("/v1/admin/seo/analyze", { path: location.pathname }, { params: { lng: locale } });
      setPage(data);
    } catch (err) {
      setPage(null);
      setError(err?.response?.status === 404 ? t("seo.notAnalysable") : errMsg(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setPage(null);
    setError(null);
    if (open) load();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname, open, locale]);

  const editable = page && (page.kind === "product" || page.kind === "category" || page.kind === "tag");
  const apply = async (field, value) => {
    try {
      await api.put(endpointOf(page), { ...page.meta, [field]: value }, { params: { lng: locale } });
      toast.success(t("admin.saved"));
      load();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <>
      <button type="button" className="seo-audit-fab" onClick={() => setOpen(true)} aria-haspopup="dialog" title={t("seo.analyzePage")}>
        <FiTrendingUp aria-hidden="true" />
        <span>{t("seo.analyzePage")}</span>
      </button>
      {open && (
        <div className="seo-drawer-backdrop" onMouseDown={(e) => e.target === e.currentTarget && setOpen(false)}>
          <aside className="seo-drawer" role="dialog" aria-modal="true" aria-label={t("seo.analyzePage")}>
            <div className="seo-drawer-head">
              <h2>{t("seo.analyzePage")}</h2>
              <button type="button" className="icon-btn" onClick={() => setOpen(false)} aria-label={t("common.close")}>
                <FiX />
              </button>
            </div>
            <div className="seo-drawer-body stack">
              {loading && !page && <Spinner />}
              {error && <ErrorBox error={error} />}
              {page && (
                <>
                  <div className="row wrap">
                    <strong className="grow">{page.name}</strong>
                    {page.editPath && (
                      <Link to={page.editPath} className="btn btn-secondary btn-sm">
                        <FiEdit2 aria-hidden="true" /> {t("seo.edit")}
                      </Link>
                    )}
                  </div>
                  <ScoreBadges analysis={page.analysis} />
                  {page.meta?.focusKeyphrase ? (
                    <p className="small">
                      {t("seo.focusKeyphrase")} : <strong>{page.meta.focusKeyphrase}</strong>
                    </p>
                  ) : (
                    editable && <p className="notice notice-warning small">{t("seo.noKeyphrase")}</p>
                  )}
                  <SnippetPreview head={page.head} path={page.path} />
                  <Tabs
                    tabs={[
                      { id: "seo", label: (<><ScoreDot rating={page.analysis?.seo?.rating} /> {t("seo.seoTab")}</>) },
                      { id: "readability", label: (<><ScoreDot rating={page.analysis?.readability?.rating} /> {t("seo.readability")}</>) },
                      { id: "ai", label: t("seo.aiTitle") },
                    ]}
                    active={tab}
                    onChange={setTab}
                  />
                  {tab === "seo" && <AnalysisList report={page.analysis?.seo} />}
                  {tab === "readability" && <AnalysisList report={page.analysis?.readability} />}
                  {tab === "ai" && (
                    <AiSuggestions
                      target={() => ({ kind: page.kind, id: page.id || 0 })}
                      onApply={editable ? apply : undefined}
                      canApply={(f) => META_FIELDS.includes(f)}
                    />
                  )}
                </>
              )}
            </div>
          </aside>
        </div>
      )}
    </>
  );
};

export default SeoPageAudit;
