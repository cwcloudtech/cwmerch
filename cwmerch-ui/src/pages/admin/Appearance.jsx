import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { FiArrowDown, FiArrowUp, FiPlus, FiRotateCcw, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { applyAppearance, COLOR_KEYS, cssVarsFor, GOOGLE_FONTS, loadGoogleFonts, SYSTEM_FONTS } from "../../lib/appearance";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import HeroSlider from "../../components/HeroSlider";
import { PageHead, uploadMedia, UploadButton } from "../../components/admin";
import { ErrorBox, Field, SelectField, Spinner, TextArea, TextField, Toggle } from "../../components/ui";

const SECTION_TYPES = ["categories", "featured", "latest", "onSale", "bestSellers", "text"];
const SOCIALS = ["facebook", "instagram", "x", "tiktok", "youtube", "whatsapp", "pinterest"];
const FONT_CHOICES = [...GOOGLE_FONTS, ...SYSTEM_FONTS.filter((f) => !["serif", "sans-serif", "monospace"].includes(f))];

const DEFAULTS = {
  name: "",
  tagline: "",
  logo: null,
  favicon: "",
  theme: "",
  colors: {},
  fonts: { heading: "", body: "" },
  radius: 8,
  banner: { enabled: false, autoplay: true, intervalSeconds: 6, height: "medium", slides: [] },
  announcement: { enabled: false, text: "", link: "" },
  homeSections: [],
  social: {},
  contact: { phone: "", email: "", address: "" },
  footerText: "",
  customCss: "",
};

const EMPTY_DETECTED = { name: "", tagline: "", colors: {}, fonts: {}, sources: {} };
const HEX6 = /^#[0-9a-f]{6}$/i;

// effective is what the storefront shows: each override, else the detected
// value (Divi, WooCommerce, WordPress, else cwmerch's default).
const effective = (a, d) => {
  const colors = {};
  COLOR_KEYS.forEach(([k]) => {
    colors[k] = (a.colors?.[k] || "").trim() || d.colors?.[k] || "";
  });
  return {
    ...a,
    name: a.name || d.name,
    tagline: a.tagline || d.tagline,
    colors,
    fonts: { heading: a.fonts?.heading || d.fonts?.heading || "", body: a.fonts?.body || d.fonts?.body || "" },
  };
};

// SourceBadge says where an inherited value comes from.
const SourceBadge = ({ source }) => {
  const { t } = useI18n();
  if (!source) return null;
  return <span className={`source-badge source-${source}`}>{t(`appearance.source.${source}`)}</span>;
};

// ColorOverride edits one color override; empty = the detected value (shown
// as the placeholder, with its source).
const ColorOverride = ({ name, value, detected, source, onChange }) => {
  const { t } = useI18n();
  const id = `color-${name}`;
  const own = (value || "").trim();
  const shown = own || detected || "#000000";
  return (
    <div className={`color-field ${own ? "is-overridden" : ""}`}>
      <input type="color" aria-label={t(`appearance.colors.${name}`)} value={HEX6.test(shown) ? shown : "#000000"} onChange={(e) => onChange(e.target.value)} />
      <div className="color-field-main">
        <label htmlFor={id}>{t(`appearance.colors.${name}`)}</label>
        <div className="row">
          <input id={id} className="input input-sm mono" value={value || ""} placeholder={detected || ""} onChange={(e) => onChange(e.target.value)} spellCheck={false} />
          {own ? (
            <button type="button" className="icon-btn" onClick={() => onChange("")} title={t("appearance.useDetected")} aria-label={t("appearance.useDetected")}>
              <FiRotateCcw />
            </button>
          ) : (
            <SourceBadge source={source} />
          )}
        </div>
      </div>
    </div>
  );
};

// The preset made of the detected look (see the API's woo.WordPressThemeID).
const DETECTED_THEME = "wordpress";

const fromServer = (data) => {
  const { detected: _detected, ...rest } = data || {};
  return {
    ...DEFAULTS,
    ...rest,
    colors: { ...(rest.colors || {}) },
    fonts: { heading: "", body: "", ...(rest.fonts || {}) },
    banner: { ...DEFAULTS.banner, ...rest.banner },
    announcement: { ...DEFAULTS.announcement, ...rest.announcement },
    contact: { ...DEFAULTS.contact, ...rest.contact },
  };
};

const NEW_SLIDE = { image: "", mobileImage: "", title: "", subtitle: "", buttonText: "", buttonLink: "/shop", align: "center", overlay: 30, textColor: "#ffffff" };

const move = (list, i, d) => {
  const j = i + d;
  if (j < 0 || j >= list.length) return list;
  const next = [...list];
  [next[i], next[j]] = [next[j], next[i]];
  return next;
};

const ImageInput = ({ label, value, onChange }) => {
  const { t, errMsg } = useI18n();
  return (
    <Field label={label}>
      {(id) => (
        <div className="row wrap">
          {value ? <img src={value} alt="" className="thumb-md" /> : <span className="img-placeholder thumb-md" />}
          <input id={id} className="input grow" type="url" value={value || ""} onChange={(e) => onChange(e.target.value)} placeholder="https://..." />
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

const Preview = ({ a }) => {
  const { t } = useI18n();
  const { money } = useShop();
  useEffect(() => loadGoogleFonts(a.fonts), [a.fonts]);
  return (
    <div className="storefront appearance-preview" style={cssVarsFor(a)} aria-label={t("appearance.preview")}>
      {a.announcement?.enabled && a.announcement.text && <div className="announcement">{a.announcement.text}</div>}
      <div className="site-header preview-header">
        <div className="header-row">
          <span className="brand">
            {a.logo?.src ? <img src={a.logo.src} alt="" className="brand-logo" /> : <img src="/cwmerch-logo-small.png" alt={a.name || "cwmerch"} className="brand-logo brand-logo-default" width="1229" height="590" />}
          </span>
          {a.tagline && <span className="tagline">{a.tagline}</span>}
        </div>
      </div>
      <HeroSlider banner={{ ...a.banner, enabled: true, autoplay: false }} preview />
      {(a.banner?.slides || []).length === 0 && <p className="muted small preview-note">{t("appearance.noSlides")}</p>}
      <div className="preview-body">
        <h2 className="preview-heading">{t("home.section.featured")}</h2>
        <div className="product-grid preview-grid">
          {[0, 1].map((i) => (
            <article key={i} className="product-card">
              <div className="product-card-media">
                <div className="img-placeholder" />
                {i === 0 && <span className="sale-flash">{t("product.sale")}</span>}
              </div>
              <div className="product-card-body">
                <h3 className="product-card-title">{t("appearance.sampleProduct")}</h3>
                <span className="price">
                  {i === 0 && <del>{money(39)}</del>} <ins>{money(29)}</ins>
                </span>
              </div>
              <div className="product-card-actions">
                <span className="btn btn-primary btn-sm btn-block">{t("product.addToCart")}</span>
              </div>
            </article>
          ))}
        </div>
      </div>
      <div className="site-footer preview-footer">
        <p>{a.footerText || `© ${a.name || "cwmerch"}`}</p>
      </div>
    </div>
  );
};

const Appearance = () => {
  const { t, errMsg } = useI18n();
  const { setAppearance } = useShop();
  const [a, setA] = useState(null);
  const [detected, setDetected] = useState(EMPTY_DETECTED);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const { data: themes } = useAsync(async () => asArray((await api.get("/v1/shop/themes")).data), [], []);

  useEffect(() => {
    api
      .get("/v1/admin/appearance")
      .then(({ data }) => {
        setDetected({ ...EMPTY_DETECTED, ...data?.detected });
        setA(fromServer(data));
      })
      .catch(setError);
  }, []);

  if (!a) return error ? <ErrorBox error={error} /> : <Spinner />;

  const set = (k) => (v) => setA((x) => ({ ...x, [k]: v }));
  const setIn = (k, sub) => (v) => setA((x) => ({ ...x, [k]: { ...(x[k] || {}), [sub]: v } }));
  const slides = a.banner.slides || [];
  const setSlides = (next) => setA((x) => ({ ...x, banner: { ...x.banner, slides: next } }));
  const setSlide = (i, patch) => setSlides(slides.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const sections = a.homeSections || [];
  const setSections = (next) => set("homeSections")(next);
  const setSection = (i, patch) => setSections(sections.map((s, j) => (j === i ? { ...s, ...patch } : s)));

  // A preset overrides every color and font; the detected one ("wordpress")
  // clears the overrides so everything is inherited again.
  const pickTheme = (th) =>
    setA((x) =>
      th.id === DETECTED_THEME ? { ...x, theme: "", colors: {}, fonts: { heading: "", body: "" } } : { ...x, theme: th.id, colors: { ...th.colors }, fonts: { ...th.fonts } }
    );
  const activeTheme = a.theme || DETECTED_THEME;
  const view = effective(a, detected);

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const body = { ...a, radius: Number(a.radius || 0), banner: { ...a.banner, intervalSeconds: Number(a.banner.intervalSeconds || 6) } };
      // Only the overrides are sent and stored (empty = inherited); the
      // detected values are never written.
      const { data } = await api.put("/v1/admin/appearance", body);
      if (data && typeof data === "object") {
        setA(fromServer(data));
        if (data.detected) setDetected({ ...EMPTY_DETECTED, ...data.detected });
      }
      const { data: served } = await api.get("/v1/shop/appearance");
      setAppearance(served);
      applyAppearance(served);
      toast.success(t("admin.saved"));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.appearance")} subtitle={t("appearance.intro")} />
      <form className="appearance-layout" onSubmit={save}>
        <div className="stack-lg">
          <section className="card stack">
            <h2>{t("appearance.identity")}</h2>
            <div className="form-grid">
              <TextField label={t("appearance.name")} value={a.name} onChange={set("name")} placeholder={detected.name} />
              <TextField label={t("appearance.tagline")} value={a.tagline} onChange={set("tagline")} placeholder={detected.tagline} />
            </div>
            <ImageInput label={t("appearance.logo")} value={a.logo?.src || ""} onChange={(src) => set("logo")(src ? { id: a.logo?.id || 0, src } : null)} />
            <ImageInput label={t("appearance.favicon")} value={a.favicon} onChange={set("favicon")} />
          </section>

          <section className="card stack">
            <h2>{t("appearance.theme")}</h2>
            <p className="muted small">{t("appearance.inheritHint")}</p>
            <div className="theme-grid" role="radiogroup" aria-label={t("appearance.theme")}>
              {themes.map((th) => (
                <label key={th.id} className={`theme-swatch ${activeTheme === th.id ? "is-active" : ""}`}>
                  <input type="radio" name="theme" className="sr-only" checked={activeTheme === th.id} onChange={() => pickTheme(th)} />
                  <span className="swatch-colors" aria-hidden="true">
                    {["primary", "secondary", "accent", "background", "text"].map((k) => (
                      <span key={k} style={{ background: th.colors?.[k] }} />
                    ))}
                  </span>
                  <span>{th.id === DETECTED_THEME ? t("appearance.detectedTheme", { sources: th.name }) : th.name}</span>
                </label>
              ))}
              {themes.length === 0 && <p className="muted small">{t("appearance.noThemes")}</p>}
            </div>
            <div className="color-grid">
              {COLOR_KEYS.map(([key]) => (
                <ColorOverride
                  key={key}
                  name={key}
                  value={a.colors?.[key]}
                  detected={detected.colors?.[key]}
                  source={detected.sources?.[`colors.${key}`]}
                  onChange={setIn("colors", key)}
                />
              ))}
            </div>
            <div className="form-grid">
              <Field label={t("appearance.headingFont")}>
                {(id) => (
                  <div className="row">
                    <input id={id} className="input" list="font-list" value={a.fonts?.heading || ""} placeholder={detected.fonts?.heading || ""} onChange={(e) => setIn("fonts", "heading")(e.target.value)} />
                    {!a.fonts?.heading && <SourceBadge source={detected.sources?.["fonts.heading"]} />}
                  </div>
                )}
              </Field>
              <Field label={t("appearance.bodyFont")}>
                {(id) => (
                  <div className="row">
                    <input id={id} className="input" list="font-list" value={a.fonts?.body || ""} placeholder={detected.fonts?.body || ""} onChange={(e) => setIn("fonts", "body")(e.target.value)} />
                    {!a.fonts?.body && <SourceBadge source={detected.sources?.["fonts.body"]} />}
                  </div>
                )}
              </Field>
              <datalist id="font-list">
                {FONT_CHOICES.map((f) => (
                  <option key={f} value={f} />
                ))}
              </datalist>
              <TextField label={t("appearance.radius")} type="number" min="0" max="32" value={a.radius} onChange={set("radius")} />
            </div>
          </section>

          <section className="card stack">
            <div className="section-head">
              <h2>{t("appearance.banner")}</h2>
              <Toggle checked={!!a.banner.enabled} onChange={setIn("banner", "enabled")} label={t("appearance.bannerEnabled")} />
            </div>
            <div className="form-grid-3">
              <Toggle checked={!!a.banner.autoplay} onChange={setIn("banner", "autoplay")} label={t("appearance.autoplay")} />
              <TextField label={t("appearance.interval")} type="number" min="2" value={a.banner.intervalSeconds} onChange={setIn("banner", "intervalSeconds")} />
              <SelectField label={t("appearance.height")} value={a.banner.height} onChange={setIn("banner", "height")} options={["small", "medium", "large", "full"].map((x) => ({ value: x, label: t(`appearance.heights.${x}`) }))} />
            </div>
            {slides.map((s, i) => (
              <fieldset key={i} className="fieldset stack">
                <legend>{t("appearance.slideN", { n: i + 1 })}</legend>
                <div className="row end">
                  <button type="button" className="icon-btn" onClick={() => setSlides(move(slides, i, -1))} disabled={i === 0} aria-label={t("common.moveUp")}>
                    <FiArrowUp />
                  </button>
                  <button type="button" className="icon-btn" onClick={() => setSlides(move(slides, i, 1))} disabled={i === slides.length - 1} aria-label={t("common.moveDown")}>
                    <FiArrowDown />
                  </button>
                  <button type="button" className="icon-btn danger" onClick={() => setSlides(slides.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                    <FiTrash2 />
                  </button>
                </div>
                <ImageInput label={t("appearance.slideImage")} value={s.image} onChange={(image) => setSlide(i, { image })} />
                <ImageInput label={t("appearance.slideMobileImage")} value={s.mobileImage} onChange={(mobileImage) => setSlide(i, { mobileImage })} />
                <div className="form-grid">
                  <TextField label={t("appearance.slideTitle")} value={s.title} onChange={(title) => setSlide(i, { title })} />
                  <TextField label={t("appearance.slideSubtitle")} value={s.subtitle} onChange={(subtitle) => setSlide(i, { subtitle })} />
                  <TextField label={t("appearance.buttonText")} value={s.buttonText} onChange={(buttonText) => setSlide(i, { buttonText })} />
                  <TextField label={t("appearance.buttonLink")} value={s.buttonLink} onChange={(buttonLink) => setSlide(i, { buttonLink })} />
                  <SelectField label={t("appearance.align")} value={s.align || "center"} onChange={(align) => setSlide(i, { align })} options={["left", "center", "right"].map((x) => ({ value: x, label: t(`appearance.aligns.${x}`) }))} />
                  <Field label={t("appearance.overlay", { n: s.overlay ?? 0 })}>
                    {(id) => <input id={id} type="range" min="0" max="90" step="5" value={s.overlay ?? 0} onChange={(e) => setSlide(i, { overlay: Number(e.target.value) })} />}
                  </Field>
                  <label className="color-field">
                    <input type="color" value={s.textColor || "#ffffff"} onChange={(e) => setSlide(i, { textColor: e.target.value })} />
                    <span>{t("appearance.textColor")}</span>
                  </label>
                </div>
              </fieldset>
            ))}
            <div>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => setSlides([...slides, { ...NEW_SLIDE }])}>
                <FiPlus aria-hidden="true" /> {t("appearance.addSlide")}
              </button>
            </div>
          </section>

          <section className="card stack">
            <div className="section-head">
              <h2>{t("appearance.announcement")}</h2>
              <Toggle checked={!!a.announcement.enabled} onChange={setIn("announcement", "enabled")} label={t("appearance.show")} />
            </div>
            <div className="form-grid">
              <TextField label={t("appearance.announcementText")} value={a.announcement.text} onChange={setIn("announcement", "text")} />
              <TextField label={t("appearance.announcementLink")} value={a.announcement.link} onChange={setIn("announcement", "link")} />
            </div>
          </section>

          <section className="card stack">
            <h2>{t("appearance.homeSections")}</h2>
            <p className="muted small">{t("appearance.homeSectionsHint")}</p>
            {sections.map((s, i) => (
              <div key={i} className="card-inner stack">
                <div className="row wrap align-end">
                  <SelectField label={t("appearance.sectionType")} value={s.type} onChange={(type) => setSection(i, { type })} options={SECTION_TYPES.map((x) => ({ value: x, label: t(`home.section.${x}`) }))} />
                  <TextField label={t("appearance.sectionTitle")} value={s.title} onChange={(title) => setSection(i, { title })} placeholder={t(`home.section.${s.type}`)} className="grow" />
                  {s.type !== "text" && <TextField label={t("appearance.limit")} type="number" min="1" max="48" value={s.limit ?? 8} onChange={(limit) => setSection(i, { limit: Number(limit) })} />}
                  <span className="row-actions">
                    <button type="button" className="icon-btn" onClick={() => setSections(move(sections, i, -1))} disabled={i === 0} aria-label={t("common.moveUp")}>
                      <FiArrowUp />
                    </button>
                    <button type="button" className="icon-btn" onClick={() => setSections(move(sections, i, 1))} disabled={i === sections.length - 1} aria-label={t("common.moveDown")}>
                      <FiArrowDown />
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => setSections(sections.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                      <FiTrash2 />
                    </button>
                  </span>
                </div>
                {s.type === "text" && <TextArea label={t("appearance.content")} hint={t("appearance.markdownHint")} value={s.content} onChange={(content) => setSection(i, { content })} rows={4} />}
              </div>
            ))}
            <div>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => setSections([...sections, { type: "featured", title: "", limit: 8, content: "" }])}>
                <FiPlus aria-hidden="true" /> {t("appearance.addSection")}
              </button>
            </div>
          </section>

          <section className="card stack">
            <h2>{t("appearance.footerContact")}</h2>
            <div className="form-grid">
              <TextField label={t("address.phone")} value={a.contact.phone} onChange={setIn("contact", "phone")} />
              <TextField label={t("auth.email")} type="email" value={a.contact.email} onChange={setIn("contact", "email")} />
              <TextField className="span-2" label={t("appearance.address")} value={a.contact.address} onChange={setIn("contact", "address")} />
              {SOCIALS.map((k) => (
                <TextField key={k} label={t(`appearance.social.${k}`)} value={a.social?.[k] || ""} onChange={setIn("social", k)} placeholder={k === "whatsapp" ? "+33600000000" : "https://"} />
              ))}
            </div>
            <TextArea label={t("appearance.footerText")} value={a.footerText} onChange={set("footerText")} rows={2} />
          </section>

          <section className="card stack">
            <h2>{t("appearance.customCss")}</h2>
            <TextArea label={t("appearance.customCssLabel")} hint={t("appearance.customCssHint")} value={a.customCss} onChange={set("customCss")} rows={8} className="mono-area" spellCheck={false} dir="ltr" />
          </section>
        </div>
        <aside className="appearance-side">
          <div className="sticky-side stack">
            <h2>{t("appearance.preview")}</h2>
            <Preview a={view} />
            <button type="submit" className="btn btn-primary btn-block" disabled={busy}>
              {t("common.saveChanges")}
            </button>
          </div>
        </aside>
      </form>
    </div>
  );
};

export default Appearance;
