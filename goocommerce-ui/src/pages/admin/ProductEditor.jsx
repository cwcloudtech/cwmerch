import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiArrowLeft, FiArrowRight, FiExternalLink, FiTrash2, FiX } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { dateInput, dateToApi, idsToText, slugify, textToIds, textToList } from "../../lib/util";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, uploadMedia, UploadButton } from "../../components/admin";
import { ConfirmModal, ErrorBox, Field, SelectField, Spinner, Tabs, TextArea, TextField, Toggle } from "../../components/ui";
import ProductAttributes from "./ProductAttributes";
import ProductVariations from "./ProductVariations";

const NEW_PRODUCT = {
  id: 0,
  name: "",
  slug: "",
  type: "simple",
  status: "draft",
  featured: false,
  catalogVisibility: "visible",
  description: "",
  shortDescription: "",
  sku: "",
  regularPrice: "",
  salePrice: "",
  dateOnSaleFrom: null,
  dateOnSaleTo: null,
  taxStatus: "taxable",
  taxClass: "",
  manageStock: false,
  stockQuantity: null,
  stockStatus: "instock",
  backorders: "no",
  lowStockAmount: null,
  soldIndividually: false,
  weight: "",
  length: "",
  width: "",
  height: "",
  shippingClass: "",
  virtual: false,
  downloadable: false,
  downloads: [],
  downloadLimit: -1,
  downloadExpiry: -1,
  externalUrl: "",
  buttonText: "",
  purchaseNote: "",
  menuOrder: 0,
  reviewsAllowed: true,
  categories: [],
  tags: [],
  images: [],
  attributes: [],
  defaultAttributes: [],
  upsellIds: [],
  crossSellIds: [],
  groupedProducts: [],
};

// IdsField edits a list of product ids as comma-separated text.
const IdsField = ({ label, hint, value, onChange }) => {
  const [text, setText] = useState(idsToText(value));
  useEffect(() => setText(idsToText(value)), [value]);
  return <TextField label={label} hint={hint} value={text} onChange={setText} onBlur={() => onChange(textToIds(text))} />;
};

const CategoryChecklist = ({ categories, value, onChange }) => {
  const ids = value.map((c) => Number(c.id));
  const render = (parent, depth) =>
    categories
      .filter((c) => Number(c.parent || 0) === parent)
      .map((c) => (
        <li key={c.id} style={{ paddingLeft: depth * 16 }}>
          <label className="check-row">
            <input
              type="checkbox"
              checked={ids.includes(Number(c.id))}
              onChange={(e) => onChange(e.target.checked ? [...value, { id: c.id, name: c.name, slug: c.slug }] : value.filter((x) => Number(x.id) !== Number(c.id)))}
            />
            {c.name}
          </label>
          <ul className="plain-list">{render(Number(c.id), depth + 1)}</ul>
        </li>
      ));
  return <ul className="plain-list category-checklist">{render(0, 0)}</ul>;
};

const ProductEditor = () => {
  const { id } = useParams();
  const isNew = !id || id === "new";
  const navigate = useNavigate();
  const { t, errMsg } = useI18n();
  const [p, setP] = useState(isNew ? NEW_PRODUCT : null);
  const [tab, setTab] = useState("general");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [tagText, setTagText] = useState("");
  const [deleting, setDeleting] = useState(null);
  const [uploading, setUploading] = useState(false);

  const { data: categories } = useAsync(async () => asArray((await api.get("/v1/admin/categories", { params: { perPage: 100 } })).data), [], []);
  const { data: allTags } = useAsync(async () => asArray((await api.get("/v1/admin/tags", { params: { perPage: 100 } })).data), [], []);
  const { data: shippingClasses } = useAsync(async () => asArray((await api.get("/v1/admin/shipping/classes")).data), [], []);
  const { data: taxClasses } = useAsync(async () => asArray((await api.get("/v1/admin/tax-classes")).data), [], []);

  useEffect(() => {
    if (isNew) {
      setP(NEW_PRODUCT);
      setTagText("");
      return;
    }
    let cancelled = false;
    api
      .get(`/v1/admin/products/${id}`)
      .then(({ data }) => {
        if (cancelled) return;
        setP({ ...NEW_PRODUCT, ...data });
        setTagText((data.tags || []).map((x) => x.name).join(", "));
      })
      .catch((e) => !cancelled && setError(e));
    return () => {
      cancelled = true;
    };
  }, [id, isNew]);

  if (!p) return error ? <ErrorBox error={error} /> : <Spinner />;

  const set = (k) => (v) => setP((x) => ({ ...x, [k]: v }));

  const buildPayload = (overrides = {}) => {
    const tags = textToList(tagText).map((name) => {
      const existing = allTags.find((x) => x.name.toLowerCase() === name.toLowerCase());
      return existing ? { id: existing.id, name: existing.name, slug: existing.slug } : { name };
    });
    const { variations: _v, related: _r, ...rest } = p;
    return {
      ...rest,
      slug: p.slug || slugify(p.name),
      regularPrice: p.regularPrice === "" ? null : p.regularPrice,
      salePrice: p.salePrice === "" ? null : p.salePrice,
      stockQuantity: p.manageStock ? Number(p.stockQuantity || 0) : null,
      lowStockAmount: p.lowStockAmount === "" ? null : p.lowStockAmount,
      menuOrder: Number(p.menuOrder || 0),
      downloadLimit: Number(p.downloadLimit ?? -1),
      downloadExpiry: Number(p.downloadExpiry ?? -1),
      categories: p.categories.map((c) => ({ id: c.id })),
      tags,
      images: p.images.map((img, i) => ({ id: img.id, src: img.src, name: img.name, alt: img.alt, position: i })),
      ...overrides,
    };
  };

  const save = async (overrides) => {
    setBusy(true);
    setError(null);
    try {
      const body = buildPayload(overrides);
      const { data } = isNew ? await api.post("/v1/admin/products", body) : await api.put(`/v1/admin/products/${p.id}`, body);
      toast.success(t("admin.products.saved"));
      if (data?.id) {
        setP({ ...NEW_PRODUCT, ...data });
        setTagText((data.tags || []).map((x) => x.name).join(", "));
        if (isNew) navigate(`/admin/products/${data.id}`, { replace: true });
      }
    } catch (err) {
      setError(errMsg(err));
      window.scrollTo({ top: 0, behavior: "smooth" });
    } finally {
      setBusy(false);
    }
  };

  const upload = async (files) => {
    setUploading(true);
    try {
      const uploaded = [];
      for (const f of files) uploaded.push(await uploadMedia(api, f));
      setP((x) => ({ ...x, images: [...x.images, ...uploaded] }));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setUploading(false);
    }
  };

  const moveImage = (i, d) =>
    setP((x) => {
      const j = i + d;
      if (j < 0 || j >= x.images.length) return x;
      const images = [...x.images];
      [images[i], images[j]] = [images[j], images[i]];
      return { ...x, images };
    });

  const duplicate = async () => {
    try {
      const { data } = await api.post(`/v1/admin/products/${p.id}/duplicate`, {});
      toast.success(t("admin.products.duplicated"));
      if (data?.id) navigate(`/admin/products/${data.id}`);
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const remove = async () => {
    const force = deleting === "force";
    setDeleting(null);
    try {
      await api.delete(`/v1/admin/products/${p.id}`, { params: force ? { force: true } : {} });
      toast.success(force ? t("admin.deleted") : t("admin.trashed"));
      navigate("/admin/products");
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const isVariable = p.type === "variable";
  const tabs = [
    { id: "general", label: t("admin.products.tabGeneral") },
    ...(p.type !== "grouped" && p.type !== "external" ? [{ id: "inventory", label: t("admin.products.tabInventory") }] : [{ id: "inventory", label: t("admin.products.tabInventory") }]),
    ...(!p.virtual && p.type !== "grouped" && p.type !== "external" ? [{ id: "shipping", label: t("admin.products.tabShipping") }] : []),
    { id: "linked", label: t("admin.products.tabLinked") },
    { id: "attributes", label: t("admin.products.tabAttributes") },
    ...(isVariable ? [{ id: "variations", label: t("admin.products.tabVariations") }] : []),
    { id: "advanced", label: t("admin.products.tabAdvanced") },
  ];

  const downloads = p.downloads || [];
  const setDownload = (i, patch) => set("downloads")(downloads.map((d, j) => (j === i ? { ...d, ...patch } : d)));

  return (
    <div className="stack-lg">
      <PageHead title={isNew ? t("admin.nav.addProduct") : t("admin.products.edit")}>
        {!isNew && p.status === "publish" && (
          <a href={`/product/${p.slug}`} target="_blank" rel="noopener noreferrer" className="btn btn-secondary btn-sm">
            <FiExternalLink aria-hidden="true" /> {t("admin.products.view")}
          </a>
        )}
        <Link to="/admin/products" className="btn btn-secondary btn-sm">
          {t("common.back")}
        </Link>
      </PageHead>
      <ErrorBox error={error} />
      <form
        className="admin-columns"
        onSubmit={(e) => {
          e.preventDefault();
          save();
        }}
      >
        <div className="stack-lg">
          <section className="card stack">
            <TextField label={t("admin.products.name")} value={p.name} onChange={set("name")} required className="field-lg" />
            <TextField label={t("admin.products.slug")} value={p.slug} onChange={(v) => set("slug")(slugify(v))} placeholder={slugify(p.name)} />
            <TextArea label={t("admin.products.description")} hint={t("admin.products.htmlAllowed")} value={p.description} onChange={set("description")} rows={10} className="mono-area" />
          </section>

          <section className="card stack">
            <div className="section-head">
              <h2>{t("admin.products.data")}</h2>
              <div className="row wrap">
                <label className="sr-only" htmlFor="ptype">
                  {t("admin.products.type")}
                </label>
                <select id="ptype" className="input input-sm" value={p.type} onChange={(e) => set("type")(e.target.value)}>
                  {["simple", "variable", "grouped", "external"].map((x) => (
                    <option key={x} value={x}>
                      {t(`productType.${x}`)}
                    </option>
                  ))}
                </select>
                {(p.type === "simple" || p.type === "variable") && (
                  <>
                    <Toggle checked={!!p.virtual} onChange={set("virtual")} label={t("admin.products.virtual")} />
                    <Toggle checked={!!p.downloadable} onChange={set("downloadable")} label={t("admin.products.downloadable")} />
                  </>
                )}
              </div>
            </div>
            <Tabs tabs={tabs} active={tab} onChange={setTab} className="tabs-scroll" />
            <div className="tab-panel" role="tabpanel">
              {tab === "general" && (
                <div className="stack">
                  {p.type === "external" && (
                    <div className="form-grid">
                      <TextField label={t("admin.products.externalUrl")} type="url" value={p.externalUrl} onChange={set("externalUrl")} />
                      <TextField label={t("admin.products.buttonText")} value={p.buttonText} onChange={set("buttonText")} placeholder={t("product.buy")} />
                    </div>
                  )}
                  {p.type !== "variable" && p.type !== "grouped" && (
                    <div className="form-grid">
                      <TextField label={t("admin.products.regularPrice")} type="number" step="0.01" min="0" value={p.regularPrice ?? ""} onChange={set("regularPrice")} />
                      <TextField label={t("admin.products.salePrice")} type="number" step="0.01" min="0" value={p.salePrice ?? ""} onChange={set("salePrice")} />
                      <TextField label={t("admin.products.saleFrom")} type="date" value={dateInput(p.dateOnSaleFrom)} onChange={(d) => set("dateOnSaleFrom")(dateToApi(d))} />
                      <TextField label={t("admin.products.saleTo")} type="date" value={dateInput(p.dateOnSaleTo)} onChange={(d) => set("dateOnSaleTo")(dateToApi(d))} />
                    </div>
                  )}
                  {isVariable && <p className="muted small">{t("admin.products.variablePriceHint")}</p>}
                  <div className="form-grid">
                    <SelectField label={t("admin.products.taxStatus")} value={p.taxStatus} onChange={set("taxStatus")} options={["taxable", "shipping", "none"].map((x) => ({ value: x, label: t(`taxStatus.${x}`) }))} />
                    <SelectField label={t("admin.products.taxClass")} value={p.taxClass} onChange={set("taxClass")} options={[{ value: "", label: t("tax.standard") }, ...taxClasses.map((c) => ({ value: c.slug, label: c.name }))]} />
                  </div>
                  {p.downloadable && (
                    <fieldset className="fieldset stack">
                      <legend>{t("admin.products.downloadableFiles")}</legend>
                      {downloads.map((d, i) => (
                        <div key={d.id || i} className="row wrap">
                          <TextField label={t("admin.products.fileName")} value={d.name} onChange={(name) => setDownload(i, { name })} />
                          <TextField label={t("admin.products.fileUrl")} value={d.file} onChange={(file) => setDownload(i, { file })} className="grow" />
                          <button type="button" className="icon-btn danger" onClick={() => set("downloads")(downloads.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                            <FiTrash2 />
                          </button>
                        </div>
                      ))}
                      <div className="row wrap">
                        <button type="button" className="btn btn-secondary btn-sm" onClick={() => set("downloads")([...downloads, { id: "", name: "", file: "" }])}>
                          {t("admin.products.addFile")}
                        </button>
                        <UploadButton
                          label={t("admin.products.uploadFile")}
                          accept="*/*"
                          onFiles={async ([f]) => {
                            try {
                              const m = await uploadMedia(api, f);
                              set("downloads")([...downloads, { id: "", name: m.name || f.name, file: m.src }]);
                            } catch (err) {
                              toast.error(errMsg(err));
                            }
                          }}
                        />
                      </div>
                      <div className="form-grid">
                        <TextField label={t("admin.products.downloadLimit")} hint={t("admin.products.unlimitedHint")} type="number" value={p.downloadLimit ?? -1} onChange={set("downloadLimit")} />
                        <TextField label={t("admin.products.downloadExpiry")} hint={t("admin.products.unlimitedHint")} type="number" value={p.downloadExpiry ?? -1} onChange={set("downloadExpiry")} />
                      </div>
                    </fieldset>
                  )}
                </div>
              )}
              {tab === "inventory" && (
                <div className="stack">
                  <TextField label={t("product.sku")} value={p.sku} onChange={set("sku")} />
                  {p.type !== "grouped" && p.type !== "external" && <Toggle checked={!!p.manageStock} onChange={set("manageStock")} label={t("admin.products.manageStock")} />}
                  {p.manageStock ? (
                    <div className="form-grid">
                      <TextField label={t("admin.products.stockQuantity")} type="number" step="1" value={p.stockQuantity ?? 0} onChange={set("stockQuantity")} />
                      <SelectField label={t("admin.products.backorders")} value={p.backorders} onChange={set("backorders")} options={["no", "notify", "yes"].map((x) => ({ value: x, label: t(`backorders.${x}`) }))} />
                      <TextField label={t("admin.products.lowStock")} type="number" value={p.lowStockAmount ?? ""} onChange={set("lowStockAmount")} />
                    </div>
                  ) : (
                    <SelectField label={t("admin.products.stockStatus")} value={p.stockStatus} onChange={set("stockStatus")} options={["instock", "outofstock", "onbackorder"].map((x) => ({ value: x, label: t(`stock.${x}`) }))} />
                  )}
                  <Toggle checked={!!p.soldIndividually} onChange={set("soldIndividually")} label={t("admin.products.soldIndividually")} />
                </div>
              )}
              {tab === "shipping" && (
                <div className="form-grid">
                  <TextField label={t("admin.products.weight")} value={p.weight} onChange={set("weight")} />
                  <TextField label={t("admin.products.length")} value={p.length} onChange={set("length")} />
                  <TextField label={t("admin.products.width")} value={p.width} onChange={set("width")} />
                  <TextField label={t("admin.products.height")} value={p.height} onChange={set("height")} />
                  <SelectField
                    label={t("admin.products.shippingClass")}
                    value={p.shippingClass}
                    onChange={set("shippingClass")}
                    options={[{ value: "", label: t("admin.products.noShippingClass") }, ...shippingClasses.map((c) => ({ value: c.slug, label: c.name }))]}
                  />
                </div>
              )}
              {tab === "linked" && (
                <div className="stack">
                  {p.type === "grouped" && (
                    <IdsField label={t("admin.products.groupedProducts")} hint={t("admin.products.idsHint")} value={p.groupedProducts} onChange={set("groupedProducts")} />
                  )}
                  <IdsField label={t("admin.products.upsells")} hint={t("admin.products.idsHint")} value={p.upsellIds} onChange={set("upsellIds")} />
                  <IdsField label={t("admin.products.crossSells")} hint={t("admin.products.idsHint")} value={p.crossSellIds} onChange={set("crossSellIds")} />
                </div>
              )}
              {tab === "attributes" && <ProductAttributes attributes={p.attributes} onChange={set("attributes")} isVariable={isVariable} />}
              {tab === "variations" && (
                <div className="stack">
                  {(p.attributes || []).some((a) => a.variation) && (
                    <fieldset className="fieldset">
                      <legend>{t("admin.variations.defaultForm")}</legend>
                      <div className="row wrap">
                        {(p.attributes || [])
                          .filter((a) => a.variation)
                          .map((a) => (
                            <SelectField
                              key={a.slug || a.name}
                              label={a.name}
                              value={(p.defaultAttributes || []).find((d) => (d.slug || d.name) === (a.slug || a.name))?.option || ""}
                              placeholder={t("admin.variations.noDefault")}
                              options={(a.options || []).map((o) => ({ value: o, label: o }))}
                              onChange={(option) =>
                                set("defaultAttributes")([
                                  ...(p.defaultAttributes || []).filter((d) => (d.slug || d.name) !== (a.slug || a.name)),
                                  ...(option ? [{ slug: a.slug, name: a.name, option }] : []),
                                ])
                              }
                            />
                          ))}
                      </div>
                    </fieldset>
                  )}
                  <p className="muted small">{t("admin.variations.saveAttributesFirst")}</p>
                  <ProductVariations product={p} shippingClasses={shippingClasses} taxClasses={taxClasses} />
                </div>
              )}
              {tab === "advanced" && (
                <div className="stack">
                  <TextArea label={t("admin.products.purchaseNote")} value={p.purchaseNote} onChange={set("purchaseNote")} rows={3} />
                  <TextField label={t("admin.products.menuOrder")} type="number" value={p.menuOrder} onChange={set("menuOrder")} />
                  <Toggle checked={p.reviewsAllowed !== false} onChange={set("reviewsAllowed")} label={t("admin.products.reviewsAllowed")} />
                </div>
              )}
            </div>
          </section>

          <section className="card">
            <TextArea label={t("admin.products.shortDescription")} hint={t("admin.products.htmlAllowed")} value={p.shortDescription} onChange={set("shortDescription")} rows={4} className="mono-area" />
          </section>
        </div>

        <aside className="stack-lg">
          <section className="card stack">
            <h2>{t("admin.products.publish")}</h2>
            <SelectField label={t("admin.products.status")} value={p.status} onChange={set("status")} options={["publish", "draft", "pending", "private"].map((x) => ({ value: x, label: t(`productStatus.${x}`) }))} />
            <SelectField
              label={t("admin.products.visibility")}
              value={p.catalogVisibility}
              onChange={set("catalogVisibility")}
              options={["visible", "catalog", "search", "hidden"].map((x) => ({ value: x, label: t(`visibility.${x}`) }))}
            />
            <Toggle checked={!!p.featured} onChange={set("featured")} label={t("admin.products.featured")} />
            <div className="row wrap">
              <button type="submit" className="btn btn-primary" disabled={busy}>
                {isNew ? t("admin.products.create") : t("common.update")}
              </button>
              {!isNew && (
                <button type="button" className="btn btn-secondary btn-sm" onClick={duplicate}>
                  {t("admin.products.duplicate")}
                </button>
              )}
            </div>
            {!isNew && (
              <div className="row wrap">
                {p.status !== "trash" ? (
                  <button type="button" className="link-btn danger" onClick={() => setDeleting("trash")}>
                    {t("admin.trash")}
                  </button>
                ) : null}
                <button type="button" className="link-btn danger" onClick={() => setDeleting("force")}>
                  {t("admin.deletePermanently")}
                </button>
              </div>
            )}
          </section>

          <section className="card stack">
            <h2>{t("admin.products.images")}</h2>
            <p className="muted small">{t("admin.products.imagesHint")}</p>
            <ul className="image-list">
              {p.images.map((img, i) => (
                <li key={`${img.id}-${i}`} className={i === 0 ? "is-featured" : ""}>
                  <img src={img.src} alt={img.alt || ""} />
                  {i === 0 && <span className="badge">{t("admin.products.featuredImage")}</span>}
                  <div className="image-actions">
                    <button type="button" className="icon-btn" onClick={() => moveImage(i, -1)} disabled={i === 0} aria-label={t("common.moveUp")}>
                      <FiArrowLeft />
                    </button>
                    <button type="button" className="icon-btn" onClick={() => moveImage(i, 1)} disabled={i === p.images.length - 1} aria-label={t("common.moveDown")}>
                      <FiArrowRight />
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => set("images")(p.images.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                      <FiX />
                    </button>
                  </div>
                  <label className="sr-only" htmlFor={`alt-${i}`}>
                    {t("admin.products.altText")}
                  </label>
                  <input
                    id={`alt-${i}`}
                    className="input input-sm"
                    placeholder={t("admin.products.altText")}
                    value={img.alt || ""}
                    onChange={(e) => set("images")(p.images.map((x, j) => (j === i ? { ...x, alt: e.target.value } : x)))}
                  />
                </li>
              ))}
            </ul>
            <UploadButton label={uploading ? t("common.loading") : t("admin.products.addImages")} multiple onFiles={upload} disabled={uploading} />
          </section>

          <section className="card stack">
            <h2>{t("product.categories")}</h2>
            <CategoryChecklist categories={categories} value={p.categories} onChange={set("categories")} />
            <Link to="/admin/categories" className="small">
              {t("admin.products.manageCategories")}
            </Link>
          </section>

          <section className="card stack">
            <h2>{t("product.tags")}</h2>
            <Field label={t("admin.products.tagsLabel")} hint={t("admin.products.tagsHint")}>
              {(fid) => (
                <>
                  <input id={fid} className="input" list="tag-suggestions" value={tagText} onChange={(e) => setTagText(e.target.value)} />
                  <datalist id="tag-suggestions">
                    {allTags.map((x) => (
                      <option key={x.id} value={`${tagText.replace(/[^,]*$/, "").trim()} ${x.name}`.trim()} />
                    ))}
                  </datalist>
                </>
              )}
            </Field>
          </section>
        </aside>
      </form>
      <ConfirmModal
        show={!!deleting}
        title={deleting === "force" ? t("admin.deletePermanently") : t("admin.trash")}
        body={t(deleting === "force" ? "admin.products.deleteBody" : "admin.products.trashBody", { name: p.name })}
        onConfirm={remove}
        onCancel={() => setDeleting(null)}
        confirmLabel={deleting === "force" ? t("admin.deletePermanently") : t("admin.trash")}
      />
    </div>
  );
};

export default ProductEditor;
