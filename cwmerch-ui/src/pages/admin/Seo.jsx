import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiEdit2, FiExternalLink, FiTrendingUp } from "react-icons/fi";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { ErrorBox, Pagination, SelectField, Spinner, Tabs, TextArea, TextField, Toggle } from "../../components/ui";
import { ScoreDot, TemplateField, VARIABLES } from "../../components/seo/SeoPanel";
import SeoEditModal from "../../components/seo/SeoEditModal";

const TYPES = ["product", "product_cat", "product_tag", "pages"];
const KIND_OF = { product: "product", product_cat: "category", product_tag: "tag" };
const RATINGS = ["", "bad", "ok", "good", "na"];
// The default templates (used when a field is left empty).
const DEFAULTS = {
  homeTitle: "%%sitename%% %%page%% %%sep%% %%sitedesc%%",
  productTitle: "%%title%% %%page%% %%sep%% %%sitename%%",
  shopTitle: "%%pt_plural%% %%page%% %%sep%% %%sitename%%",
  categoryTitle: "%%term_title%% %%page%% %%sep%% %%sitename%%",
  tagTitle: "%%term_title%% %%page%% %%sep%% %%sitename%%",
};

const Overview = () => {
  const { t, locale } = useI18n();
  const [params, setParams] = useSearchParams();
  const type = TYPES.includes(params.get("type")) ? params.get("type") : "product";
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [rating, setRating] = useState("");
  const [editing, setEditing] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/seo/overview", { params: { type, page, perPage: 20, search: search || undefined, lng: locale } })).data, page),
    [type, page, search, locale],
    { items: [], totalPages: 1 }
  );
  const items = rating ? data.items.filter((r) => (r.seoRating || "na") === rating) : data.items;
  return (
    <div className="stack">
      <div className="toolbar">
        <div className="row wrap">
          <SelectField
            label={t("seo.contentType")}
            value={type}
            onChange={(v) => {
              setParams((p) => {
                const n = new URLSearchParams(p);
                n.set("type", v);
                return n;
              });
              setPage(1);
            }}
            options={TYPES.map((x) => ({ value: x, label: t(`seo.types.${x}`) }))}
          />
          <SelectField label={t("seo.seoScore")} value={rating} onChange={setRating} options={RATINGS.map((r) => ({ value: r, label: r ? t(`seo.ratings.${r}`) : t("seo.allScores") }))} />
        </div>
        {type !== "pages" && (
          <SearchBox
            value={search}
            onSearch={(s) => {
              setSearch(s);
              setPage(1);
            }}
          />
        )}
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table seo-table">
            <thead>
              <tr>
                <th scope="col">{t("seo.name")}</th>
                <th scope="col" className="center">
                  <abbr title={t("seo.seoAnalysis")}>SEO</abbr>
                </th>
                <th scope="col" className="center">
                  <abbr title={t("seo.readability")}>{t("seo.readabilityShort")}</abbr>
                </th>
                <th scope="col">{t("seo.focusKeyphrase")}</th>
                <th scope="col">{t("seo.seoTitle")}</th>
                <th scope="col">{t("seo.metaDescription")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {items.map((r) => (
                <tr key={`${r.kind}-${r.id}`}>
                  <td>
                    <strong>{r.name}</strong>
                    {r.cornerstone && <span className="badge badge-processing">{t("seo.cornerstoneShort")}</span>}
                    {r.noIndex && <span className="badge badge-pending">noindex</span>}
                    {r.status && r.status !== "publish" && <span className="muted small"> — {t(`productStatus.${r.status}`)}</span>}
                  </td>
                  <td className="center">
                    <ScoreDot rating={r.seoRating} title={`${t("seo.seoAnalysis")}: ${t(`seo.ratings.${r.seoRating || "na"}`)}${r.seoScore >= 0 ? ` (${r.seoScore}/100)` : ""}`} />
                  </td>
                  <td className="center">
                    <ScoreDot rating={r.readabilityRating} title={`${t("seo.readability")}: ${t(`seo.ratings.${r.readabilityRating || "na"}`)}`} />
                  </td>
                  <td className="small">{r.focusKeyphrase || <span className="muted">—</span>}</td>
                  <td className="small">{r.title}</td>
                  <td className="small">{r.descriptionIsAuto ? <em className="muted">{r.description || "—"}</em> : r.description}</td>
                  <td className="row-actions">
                    {r.kind === "home" || r.kind === "shop" ? (
                      <Link className="icon-btn" to="/admin/seo?tab=settings" aria-label={t("seo.editSeo")} title={t("seo.editSeo")}>
                        <FiEdit2 />
                      </Link>
                    ) : (
                      <button type="button" className="icon-btn" onClick={() => setEditing(r)} aria-label={t("seo.editSeo")} title={t("seo.editSeo")}>
                        <FiTrendingUp />
                      </button>
                    )}
                    {r.kind === "product" && (
                      <Link className="icon-btn" to={`/admin/products/${r.id}`} aria-label={t("admin.products.edit")} title={t("admin.products.edit")}>
                        <FiEdit2 />
                      </Link>
                    )}
                    {r.path && (
                      <a className="icon-btn" href={r.path} target="_blank" rel="noreferrer" aria-label={t("seo.viewPage")} title={t("seo.viewPage")}>
                        <FiExternalLink />
                      </a>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {items.length === 0 && <p className="muted">{t("seo.none")}</p>}
        </div>
      )}
      {type !== "pages" && <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />}
      {editing && (
        <SeoEditModal
          kind={editing.kind || KIND_OF[type]}
          id={editing.id}
          name={editing.name}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            reload();
          }}
        />
      )}
    </div>
  );
};

const Settings = () => {
  const { t, errMsg } = useI18n();
  const [s, setS] = useState(null);
  const [seps, setSeps] = useState([]);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api
      .get("/v1/admin/seo/settings")
      .then(({ data }) => {
        setS(data.settings);
        setSeps(data.separators || []);
      })
      .catch(setError);
  }, []);
  if (!s) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setS((x) => ({ ...x, [k]: v }));
  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const { data } = await api.put("/v1/admin/seo/settings", s);
      setS(data.settings);
      toast.success(t("admin.saved"));
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };
  const termVars = VARIABLES.filter((v) => !["title", "excerpt", "primary_category", "wc_price", "wc_sku", "wc_shortdesc"].includes(v));
  const siteVars = ["sitename", "sitedesc", "sep", "pt_plural", "currentyear", "currentmonth"];
  return (
    <form className="stack-lg" onSubmit={save}>
      <section className="card stack">
        <h2>{t("seo.settings.general")}</h2>
        <fieldset className="fieldset">
          <legend>{t("seo.settings.separator")}</legend>
          <div className="seo-separators" role="radiogroup">
            {seps.map((sp) => (
              <label key={sp.id} className={`seo-sep ${s.separator === sp.id ? "is-active" : ""}`}>
                <input type="radio" name="separator" className="sr-only" checked={s.separator === sp.id} onChange={() => set("separator")(sp.id)} />
                {sp.char}
              </label>
            ))}
          </div>
        </fieldset>
        <div className="form-grid">
          <TextField label={t("seo.settings.websiteName")} value={s.websiteName} onChange={set("websiteName")} hint={t("seo.settings.websiteNameHint")} />
          <TextField label={t("seo.settings.alternateName")} value={s.alternateName} onChange={set("alternateName")} />
        </div>
      </section>

      <section className="card stack">
        <h2>{t("seo.settings.homepage")}</h2>
        <TemplateField label={t("seo.seoTitle")} value={s.homeTitle} onChange={set("homeTitle")} placeholder={DEFAULTS.homeTitle} variables={siteVars} />
        <TemplateField label={t("seo.metaDescription")} value={s.homeDescription} onChange={set("homeDescription")} multiline variables={siteVars} />
        <div className="form-grid">
          <TextField label={t("seo.ogTitle")} value={s.ogHomeTitle} onChange={set("ogHomeTitle")} />
          <TextField label={t("seo.ogImage")} type="url" value={s.ogHomeImage} onChange={set("ogHomeImage")} placeholder="https://..." />
        </div>
        <TextArea label={t("seo.ogDescription")} value={s.ogHomeDescription} onChange={set("ogHomeDescription")} rows={2} />
      </section>

      <section className="card stack">
        <h2>{t("seo.types.product")}</h2>
        <TemplateField label={t("seo.settings.titleTemplate")} value={s.productTitle} onChange={set("productTitle")} placeholder={DEFAULTS.productTitle} />
        <TemplateField label={t("seo.settings.descriptionTemplate")} value={s.productDescription} onChange={set("productDescription")} multiline hint={t("seo.settings.descriptionTemplateHint")} />
        <Toggle checked={!s.productNoIndex} onChange={(v) => set("productNoIndex")(!v)} label={t("seo.settings.showInSearch")} />
        <h3>{t("seo.settings.shopPage")}</h3>
        <TemplateField label={t("seo.settings.titleTemplate")} value={s.shopTitle} onChange={set("shopTitle")} placeholder={DEFAULTS.shopTitle} variables={siteVars} />
        <TemplateField label={t("seo.settings.descriptionTemplate")} value={s.shopDescription} onChange={set("shopDescription")} multiline variables={siteVars} />
      </section>

      {[
        ["category", "product_cat"],
        ["tag", "product_tag"],
      ].map(([k, type]) => (
        <section key={k} className="card stack">
          <h2>{t(`seo.types.${type}`)}</h2>
          <TemplateField label={t("seo.settings.titleTemplate")} value={s[`${k}Title`]} onChange={set(`${k}Title`)} placeholder={DEFAULTS[`${k}Title`]} variables={termVars} />
          <TemplateField label={t("seo.settings.descriptionTemplate")} value={s[`${k}Description`]} onChange={set(`${k}Description`)} multiline variables={termVars} hint={t("seo.settings.descriptionTemplateHint")} />
          <Toggle checked={!s[`${k}NoIndex`]} onChange={(v) => set(`${k}NoIndex`)(!v)} label={t("seo.settings.showInSearch")} />
        </section>
      ))}

      <section className="card stack">
        <h2>{t("seo.settings.representation")}</h2>
        <SelectField
          label={t("seo.settings.companyOrPerson")}
          value={s.companyOrPerson}
          onChange={set("companyOrPerson")}
          options={[
            { value: "company", label: t("seo.settings.organization") },
            { value: "person", label: t("seo.settings.person") },
          ]}
        />
        {s.companyOrPerson === "person" ? (
          <TextField label={t("seo.settings.personName")} value={s.personName} onChange={set("personName")} />
        ) : (
          <div className="form-grid">
            <TextField label={t("seo.settings.companyName")} value={s.companyName} onChange={set("companyName")} />
            <TextField label={t("seo.settings.companyLogo")} type="url" value={s.companyLogo} onChange={set("companyLogo")} placeholder="https://..." />
          </div>
        )}
      </section>

      <section className="card stack">
        <h2>{t("seo.social")}</h2>
        <Toggle checked={!!s.openGraph} onChange={set("openGraph")} label={t("seo.settings.openGraph")} />
        <Toggle checked={!!s.twitter} onChange={set("twitter")} label={t("seo.settings.twitterCards")} />
        <div className="form-grid">
          <TextField label={t("seo.settings.facebookPage")} type="url" value={s.facebookSite} onChange={set("facebookSite")} placeholder="https://facebook.com/..." />
          <TextField label={t("seo.settings.twitterSite")} value={s.twitterSite} onChange={set("twitterSite")} placeholder="@..." />
        </div>
        <TextArea
          label={t("seo.settings.otherProfiles")}
          hint={t("seo.settings.otherProfilesHint")}
          value={(s.otherSocialUrls || []).join("\n")}
          onChange={(v) => set("otherSocialUrls")(v.split("\n"))}
          rows={3}
        />
        <TextField label={t("seo.settings.defaultImage")} type="url" value={s.ogDefaultImage} onChange={set("ogDefaultImage")} placeholder="https://..." />
      </section>

      <ErrorBox error={error} />
      <div className="row">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

const Seo = () => {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "settings" ? "settings" : "overview";
  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.seo")} subtitle={t("seo.subtitle")} />
      <Tabs
        tabs={[
          { id: "overview", label: t("seo.overview") },
          { id: "settings", label: t("seo.settings.title") },
        ]}
        active={tab}
        onChange={(id) => setParams(id === "settings" ? { tab: id } : {})}
      />
      {tab === "settings" ? <Settings /> : <Overview />}
    </div>
  );
};

export default Seo;
