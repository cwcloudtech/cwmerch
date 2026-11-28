import { useEffect, useRef, useState } from "react";
import { toast } from "react-toastify";
import { FiChevronDown, FiChevronRight, FiCpu, FiMonitor, FiSmartphone, FiCheck } from "react-icons/fi";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { Field, Spinner, Tabs, TextArea, TextField, Toggle, SelectField } from "../ui";
import { UploadButton, uploadMedia } from "../admin";

export const EMPTY_SEO = {
  focusKeyphrase: "",
  title: "",
  description: "",
  noIndex: "",
  noFollow: false,
  advanced: [],
  canonical: "",
  breadcrumbTitle: "",
  ogTitle: "",
  ogDescription: "",
  ogImage: "",
  twitterTitle: "",
  twitterDescription: "",
  twitterImage: "",
  cornerstone: false,
  primaryCategory: 0,
};

export const VARIABLES = ["title", "sitename", "sep", "sitedesc", "excerpt", "primary_category", "term_title", "term_description", "pt_plural", "currentyear", "currentmonth", "focuskw", "wc_price", "wc_sku", "wc_shortdesc"];

// ------------------------------------------------------------------ bits

export const ScoreDot = ({ rating, label, title }) => (
  <span className={`seo-dot seo-${rating || "na"}`} title={title || label} aria-label={title || label} role="img">
    {label && <span className="seo-dot-label">{label}</span>}
  </span>
);

export const ScoreBadges = ({ analysis }) => {
  const { t } = useI18n();
  if (!analysis) return null;
  return (
    <span className="seo-badges">
      <span className="seo-badge">
        <ScoreDot rating={analysis.seo?.rating} /> {t("seo.seoAnalysis")}: {t(`seo.ratings.${analysis.seo?.rating || "na"}`)}
      </span>
      <span className="seo-badge">
        <ScoreDot rating={analysis.readability?.rating} /> {t("seo.readability")}: {t(`seo.ratings.${analysis.readability?.rating || "na"}`)}
      </span>
    </span>
  );
};

const GROUPS = [
  { key: "bad", label: "seo.problems" },
  { key: "ok", label: "seo.improvements" },
  { key: "good", label: "seo.goodResults" },
];

export const AnalysisList = ({ report }) => {
  const { t } = useI18n();
  const [open, setOpen] = useState({ bad: true, ok: true, good: false });
  const items = report?.assessments || [];
  if (!items.length) return <p className="muted small">{t("seo.nothingToAnalyse")}</p>;
  return (
    <div className="seo-results">
      {GROUPS.map((g) => {
        const list = items.filter((a) => a.rating === g.key);
        if (!list.length) return null;
        return (
          <div key={g.key} className="seo-group">
            <button type="button" className="seo-group-head" aria-expanded={!!open[g.key]} onClick={() => setOpen((o) => ({ ...o, [g.key]: !o[g.key] }))}>
              {open[g.key] ? <FiChevronDown aria-hidden="true" /> : <FiChevronRight aria-hidden="true" />}
              {t(g.label)} ({list.length})
            </button>
            {open[g.key] && (
              <ul>
                {list.map((a) => (
                  <li key={a.id}>
                    <ScoreDot rating={a.rating} title={t(`seo.ratings.${a.rating}`)} />
                    <span>
                      <strong>{a.title}</strong> : {a.text}
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        );
      })}
    </div>
  );
};

// Progress under the title / meta description (traffic-light colours).
const Meter = ({ value, min, max, label }) => {
  const pct = Math.min(100, Math.round((value / max) * 100));
  const rating = value === 0 ? "bad" : value < min ? "ok" : value > max ? "bad" : "good";
  return (
    <div className="seo-meter" role="meter" aria-valuemin={0} aria-valuemax={max} aria-valuenow={value} aria-label={label}>
      <span className={`seo-meter-bar seo-${rating}`} style={{ width: `${pct}%` }} />
    </div>
  );
};

// titleWidth mirrors the API's estimate (Arial 20px).
export const titleWidth = (s) => {
  let w = 0;
  for (const ch of s || "") {
    if ("iljI.,:;|!'’`".includes(ch)) w += 5;
    else if ("ftr()[]{}\"-/\\ ".includes(ch)) w += 7;
    else if ("mwMW@%".includes(ch)) w += 16;
    else if (ch >= "A" && ch <= "Z") w += 13;
    else if (ch >= "0" && ch <= "9") w += 11;
    else w += 10.5;
  }
  return Math.round(w);
};

export const SnippetPreview = ({ head, path }) => {
  const { t } = useI18n();
  const [mode, setMode] = useState("desktop");
  if (!head) return null;
  let host = "";
  let crumbs = "";
  try {
    const u = new URL(head.canonical || window.location.origin + (path || "/"));
    host = u.host;
    crumbs = u.pathname.split("/").filter(Boolean).join(" › ");
  } catch {
    host = window.location.host;
  }
  const title = head.title || "";
  const desc = head.description || "";
  const cut = (s, n) => (s.length > n ? `${s.slice(0, n - 1).trimEnd()} …` : s);
  return (
    <div className="seo-snippet-wrap">
      <div className="row wrap seo-snippet-modes" role="group" aria-label={t("seo.previewAs")}>
        <span className="small muted">{t("seo.googlePreview")}</span>
        <button type="button" className={`chip ${mode === "mobile" ? "is-active" : ""}`} onClick={() => setMode("mobile")} aria-pressed={mode === "mobile"}>
          <FiSmartphone aria-hidden="true" /> {t("seo.mobile")}
        </button>
        <button type="button" className={`chip ${mode === "desktop" ? "is-active" : ""}`} onClick={() => setMode("desktop")} aria-pressed={mode === "desktop"}>
          <FiMonitor aria-hidden="true" /> {t("seo.desktop")}
        </button>
      </div>
      <div className={`seo-snippet seo-snippet-${mode}`} dir="auto">
        <div className="seo-snippet-url">
          <span className="seo-snippet-favicon" aria-hidden="true" />
          <span>
            <span className="seo-snippet-site">{host}</span>
            <span className="seo-snippet-crumbs">
              {host}
              {crumbs ? ` › ${crumbs}` : ""}
            </span>
          </span>
        </div>
        <div className="seo-snippet-title">{titleWidth(title) > 600 ? cut(title, Math.floor((title.length * 600) / titleWidth(title))) : title}</div>
        <div className="seo-snippet-desc">{cut(desc, mode === "mobile" ? 120 : 156)}</div>
      </div>
      {head.noIndex && <p className="notice notice-warning small">{t("seo.noindexWarning")}</p>}
    </div>
  );
};

// TemplateField is an SEO title / description input with "insert variable".
export const TemplateField = ({ label, value, onChange, placeholder, multiline, hint, meter, variables = VARIABLES }) => {
  const { t } = useI18n();
  const ref = useRef(null);
  const insert = (v) => {
    if (!v) return;
    const token = `%%${v}%%`;
    const el = ref.current?.querySelector("input,textarea");
    const cur = value || "";
    if (el && typeof el.selectionStart === "number") {
      const s = el.selectionStart;
      const e = el.selectionEnd;
      onChange(cur.slice(0, s) + token + cur.slice(e));
    } else {
      onChange(cur ? `${cur} ${token}` : token);
    }
  };
  return (
    <div className="seo-template" ref={ref}>
      {multiline ? (
        <TextArea label={label} value={value} onChange={onChange} placeholder={placeholder} rows={3} hint={hint} />
      ) : (
        <TextField label={label} value={value} onChange={onChange} placeholder={placeholder} hint={hint} />
      )}
      {meter}
      <label className="seo-insert">
        <span className="sr-only">{t("seo.insertVariable")}</span>
        <select className="input input-sm" value="" onChange={(e) => insert(e.target.value)}>
          <option value="">{t("seo.insertVariable")}</option>
          {variables.map((v) => (
            <option key={v} value={v}>
              {t(`seo.vars.${v}`)} (%%{v}%%)
            </option>
          ))}
        </select>
      </label>
    </div>
  );
};

const ImageField = ({ label, value, onChange, placeholder }) => {
  const { t, errMsg } = useI18n();
  return (
    <Field label={label}>
      {(id) => (
        <div className="row wrap">
          {value || placeholder ? <img src={value || placeholder} alt="" className={`thumb-md ${value ? "" : "is-inherited"}`} /> : <span className="img-placeholder thumb-md" />}
          <input id={id} className="input grow" type="url" value={value || ""} onChange={(e) => onChange(e.target.value)} placeholder={placeholder || "https://..."} />
          <UploadButton
            label={t("admin.products.uploadImage")}
            onFiles={async ([f]) => {
              try {
                onChange((await uploadMedia(api, f)).src);
              } catch (err) {
                toast.error(errMsg(err));
              }
            }}
          />
        </div>
      )}
    </Field>
  );
};

const SocialPreview = ({ network, title, description, image, host }) => (
  <div className={`seo-social seo-social-${network}`}>
    {image ? <img src={image} alt="" /> : <div className="seo-social-noimg" />}
    <div className="seo-social-body">
      <span className="seo-social-host">{host}</span>
      <strong>{title}</strong>
      <span>{description}</span>
    </div>
  </div>
);

// ------------------------------------------------------------------ AI

const FIELD_LABELS = {
  focusKeyphrase: "seo.focusKeyphrase",
  title: "seo.seoTitle",
  description: "seo.metaDescription",
  slug: "admin.products.slug",
  shortDescription: "admin.products.shortDescription",
  text: "admin.products.description",
  ogTitle: "seo.ogTitle",
  ogDescription: "seo.ogDescription",
  breadcrumbTitle: "seo.breadcrumbTitle",
};

export const AiSuggestions = ({ target, onApply, canApply = () => true }) => {
  const { t, locale, errMsg } = useI18n();
  const [busy, setBusy] = useState(false);
  const [res, setRes] = useState(null);
  const [error, setError] = useState(null);
  const [applied, setApplied] = useState({});
  const run = async () => {
    setBusy(true);
    setError(null);
    setApplied({});
    try {
      const { data } = await api.post("/v1/admin/seo/suggest", target(), { params: { lng: locale } });
      setRes(data);
    } catch (err) {
      const code = err?.response?.data?.i18n_code;
      setError(code === "errors.aiNotConfigured" || err?.response?.status === 428 ? t("seo.aiNotConfigured") : errMsg(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="seo-ai stack">
      <div className="row wrap">
        <button type="button" className="btn btn-secondary btn-sm" onClick={run} disabled={busy}>
          <FiCpu aria-hidden="true" /> {busy ? t("seo.aiRunning") : t("seo.analyzeWithAi")}
        </button>
        <span className="muted small">{t("seo.aiHint")}</span>
      </div>
      {busy && <Spinner label={t("seo.aiRunning")} />}
      {error && <p className="notice notice-warning small">{error}</p>}
      {res && !busy && (
        <div className="stack">
          {res.summary && <p className="seo-ai-summary">{res.summary}</p>}
          <ul className="seo-ai-list">
            {(res.suggestions || []).map((s, i) => (
              <li key={i} className="card-sub">
                <div className="row wrap">
                  <strong className="grow">{s.field ? t(FIELD_LABELS[s.field] || "seo.advice") : t("seo.advice")}</strong>
                  {s.field && s.value && onApply && canApply(s.field) && (
                    <button
                      type="button"
                      className="btn btn-primary btn-sm"
                      disabled={applied[i]}
                      onClick={() => {
                        onApply(s.field, s.value);
                        setApplied((a) => ({ ...a, [i]: true }));
                      }}
                    >
                      {applied[i] ? (
                        <>
                          <FiCheck aria-hidden="true" /> {t("seo.applied")}
                        </>
                      ) : (
                        t("seo.apply")
                      )}
                    </button>
                  )}
                </div>
                {s.value && (s.field === "text" || s.field === "shortDescription" ? <div className="seo-ai-value rich-content" dangerouslySetInnerHTML={{ __html: s.value }} /> : <p className="seo-ai-value">{s.value}</p>)}
                {s.reason && <p className="muted small">{s.reason}</p>}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
};

// ------------------------------------------------------------------ hook

// useSeoAnalysis runs the live analysis (debounced) of a page and its draft.
export const useSeoAnalysis = (kind, id, meta, draft, enabled = true) => {
  const { locale } = useI18n();
  const [page, setPage] = useState(null);
  const [loading, setLoading] = useState(false);
  const key = JSON.stringify({ kind, id, meta, draft, locale });
  useEffect(() => {
    if (!enabled) return undefined;
    let live = true;
    setLoading(true);
    const timer = setTimeout(async () => {
      try {
        const { data } = await api.post("/v1/admin/seo/analyze", { kind, id: id || 0, meta, draft }, { params: { lng: locale } });
        if (live) setPage(data);
      } catch {
        /* the panel keeps the last result */
      } finally {
        if (live) setLoading(false);
      }
    }, 700);
    return () => {
      live = false;
      clearTimeout(timer);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, enabled]);
  return { page, loading };
};

// ------------------------------------------------------------------ panel

// SeoPanel is the SEO box of a product or term editor: focus keyphrase,
// Google preview, SEO title / meta description, analyses, social, advanced,
// AI suggestions. meta/onChange hold the SEO data; draft is the editor's
// unsaved content; onApplyDraft(field, value) applies an AI suggestion to it.
const SeoPanel = ({ kind, id, meta, onChange, draft, onApplyDraft, categories, onAnalysis }) => {
  const { t } = useI18n();
  const [tab, setTab] = useState("seo");
  const m = { ...EMPTY_SEO, ...meta };
  const set = (k) => (v) => onChange({ ...m, [k]: v });
  const { page, loading } = useSeoAnalysis(kind, id, m, draft);
  useEffect(() => {
    if (page?.analysis && onAnalysis) onAnalysis(page.analysis);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page]);
  const head = page?.head;
  const vars = page?.vars || {};
  const resolve = (tpl) =>
    (tpl || "")
      .replace(/%%([a-z0-9_]+)%%/gi, (_, v) => vars[v.toLowerCase()] ?? "")
      .replace(/\s{2,}/g, " ")
      .trim();
  const titleShown = head?.title || "";
  const descShown = m.description ? resolve(m.description) : head?.description || "";
  const host = (() => {
    try {
      return new URL(head?.canonical || window.location.origin).host;
    } catch {
      return window.location.host;
    }
  })();
  const product = kind === "product";
  const noIndexDefault = t("seo.robotsDefault");

  const applyAi = (field, value) => {
    if (["focusKeyphrase", "title", "description", "ogTitle", "ogDescription", "breadcrumbTitle"].includes(field)) {
      onChange({ ...m, [field]: value });
    } else if (onApplyDraft) {
      onApplyDraft(field, value);
    }
  };

  return (
    <section className="card stack seo-panel" aria-labelledby={`seo-${kind}-${id}`}>
      <div className="row wrap">
        <h2 id={`seo-${kind}-${id}`} className="grow">
          {t("seo.title")}
        </h2>
        {loading && <span className="muted small">{t("seo.analysing")}</span>}
        <ScoreBadges analysis={page?.analysis} />
      </div>
      <Tabs
        tabs={[
          { id: "seo", label: (<><ScoreDot rating={page?.analysis?.seo?.rating} /> {t("seo.seoTab")}</>) },
          { id: "readability", label: (<><ScoreDot rating={page?.analysis?.readability?.rating} /> {t("seo.readability")}</>) },
          { id: "social", label: t("seo.social") },
          { id: "advanced", label: t("seo.advanced") },
        ]}
        active={tab}
        onChange={setTab}
      />
      <div className="tab-panel stack" role="tabpanel">
        {tab === "seo" && (
          <>
            <TextField label={t("seo.focusKeyphrase")} hint={t("seo.focusKeyphraseHint")} value={m.focusKeyphrase} onChange={set("focusKeyphrase")} />
            <SnippetPreview head={head ? { ...head, description: descShown } : null} />
            <TemplateField
              label={t("seo.seoTitle")}
              value={m.title}
              onChange={set("title")}
              placeholder={t("seo.templateDefault")}
              meter={<Meter value={titleWidth(titleShown)} min={400} max={600} label={t("seo.seoTitle")} />}
            />
            {draft?.slug !== undefined && (
              <TextField label={t("admin.products.slug")} value={draft.slug || ""} onChange={(v) => onApplyDraft?.("slug", v)} />
            )}
            <TemplateField
              label={t("seo.metaDescription")}
              value={m.description}
              onChange={set("description")}
              multiline
              placeholder={head?.descriptionIsAuto ? head.description : t("seo.metaDescriptionPlaceholder")}
              hint={t("seo.metaDescriptionHint", { n: descShown.length })}
              meter={<Meter value={m.description ? descShown.length : 0} min={120} max={156} label={t("seo.metaDescription")} />}
            />
            {product && categories?.length > 1 && (
              <SelectField
                label={t("seo.primaryCategory")}
                value={String(m.primaryCategory || "")}
                onChange={(v) => set("primaryCategory")(Number(v) || 0)}
                options={[{ value: "", label: t("seo.firstCategory") }, ...categories.map((c) => ({ value: String(c.id), label: c.name }))]}
              />
            )}
            <Toggle checked={!!m.cornerstone} onChange={set("cornerstone")} label={t("seo.cornerstone")} hint={t("seo.cornerstoneHint")} />
            <h3>{t("seo.seoAnalysis")}</h3>
            <AnalysisList report={page?.analysis?.seo} />
            <h3>{t("seo.aiTitle")}</h3>
            <AiSuggestions target={() => ({ kind, id: id || 0, meta: m, draft })} onApply={applyAi} canApply={(f) => !!onApplyDraft || !["slug", "shortDescription", "text"].includes(f)} />
          </>
        )}
        {tab === "readability" && (
          <>
            {page?.analysis?.flesch >= 0 && <p className="muted small">{t("seo.fleschScore", { n: page.analysis.flesch })}</p>}
            <AnalysisList report={page?.analysis?.readability} />
          </>
        )}
        {tab === "social" && (
          <div className="stack">
            <h3>Facebook / Open Graph</h3>
            <SocialPreview network="facebook" host={host} title={resolve(m.ogTitle) || titleShown} description={resolve(m.ogDescription) || descShown} image={m.ogImage || head?.ogImage} />
            <TemplateField label={t("seo.ogTitle")} value={m.ogTitle} onChange={set("ogTitle")} placeholder={titleShown} />
            <TemplateField label={t("seo.ogDescription")} value={m.ogDescription} onChange={set("ogDescription")} placeholder={descShown} multiline />
            <ImageField label={t("seo.ogImage")} value={m.ogImage} onChange={set("ogImage")} placeholder={head?.ogImage} />
            <h3>X</h3>
            <SocialPreview
              network="x"
              host={host}
              title={resolve(m.twitterTitle) || resolve(m.ogTitle) || titleShown}
              description={resolve(m.twitterDescription) || resolve(m.ogDescription) || descShown}
              image={m.twitterImage || m.ogImage || head?.twitterImage}
            />
            <TemplateField label={t("seo.twitterTitle")} value={m.twitterTitle} onChange={set("twitterTitle")} placeholder={resolve(m.ogTitle) || titleShown} />
            <TemplateField label={t("seo.twitterDescription")} value={m.twitterDescription} onChange={set("twitterDescription")} placeholder={resolve(m.ogDescription) || descShown} multiline />
            <ImageField label={t("seo.twitterImage")} value={m.twitterImage} onChange={set("twitterImage")} placeholder={m.ogImage || head?.twitterImage} />
          </div>
        )}
        {tab === "advanced" && (
          <div className="stack">
            <SelectField
              label={t("seo.allowIndex")}
              value={m.noIndex}
              onChange={set("noIndex")}
              options={[
                { value: "", label: noIndexDefault },
                { value: "index", label: t("seo.yes") },
                { value: "noindex", label: t("seo.no") },
              ]}
            />
            <Toggle checked={!m.noFollow} onChange={(v) => set("noFollow")(!v)} label={t("seo.followLinks")} />
            <fieldset className="fieldset">
              <legend>{t("seo.metaRobotsAdvanced")}</legend>
              {["noimageindex", "noarchive", "nosnippet"].map((a) => (
                <label key={a} className="check">
                  <input
                    type="checkbox"
                    checked={(m.advanced || []).includes(a)}
                    onChange={(e) => set("advanced")(e.target.checked ? [...(m.advanced || []), a] : (m.advanced || []).filter((x) => x !== a))}
                  />{" "}
                  {t(`seo.robots.${a}`)}
                </label>
              ))}
            </fieldset>
            <TextField label={t("seo.breadcrumbTitle")} value={m.breadcrumbTitle} onChange={set("breadcrumbTitle")} placeholder={draft?.name || page?.name} />
            <TextField label={t("seo.canonical")} type="url" value={m.canonical} onChange={set("canonical")} placeholder={head?.canonical} hint={t("seo.canonicalHint")} />
          </div>
        )}
      </div>
    </section>
  );
};

export default SeoPanel;
