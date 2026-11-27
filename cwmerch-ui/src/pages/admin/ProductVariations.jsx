import { useState } from "react";
import { toast } from "react-toastify";
import { FiChevronDown, FiChevronRight, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { dateInput, dateToApi } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { uploadMedia, UploadButton } from "../../components/admin";
import { ConfirmModal, ErrorBox, SelectField, Spinner, TextArea, TextField, Toggle } from "../../components/ui";

const VariationEditor = ({ product, variation, shippingClasses, taxClasses, onSaved, onDelete }) => {
  const { t, errMsg } = useI18n();
  const { money } = useShop();
  const [open, setOpen] = useState(false);
  const [v, setV] = useState(variation);
  const [busy, setBusy] = useState(false);
  const set = (k) => (val) => setV((x) => ({ ...x, [k]: val }));
  const varAttrs = (product.attributes || []).filter((a) => a.variation);
  const attrValue = (a) => (v.attributes || []).find((x) => (x.slug || x.name) === (a.slug || a.name))?.option || "";
  const setAttr = (a, option) => {
    const others = (v.attributes || []).filter((x) => (x.slug || x.name) !== (a.slug || a.name));
    setV({ ...v, attributes: [...others, { slug: a.slug, name: a.name, option }] });
  };

  const save = async () => {
    setBusy(true);
    try {
      const body = {
        ...v,
        regularPrice: v.regularPrice === "" ? null : v.regularPrice,
        salePrice: v.salePrice === "" ? null : v.salePrice,
        stockQuantity: v.manageStock ? Number(v.stockQuantity || 0) : null,
      };
      const { data } = await api.put(`/v1/admin/products/${product.id}/variations/${v.id}`, body);
      toast.success(t("admin.saved"));
      onSaved(data?.id ? data : v);
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const upload = async ([file]) => {
    try {
      const img = await uploadMedia(api, file);
      setV((x) => ({ ...x, image: { id: img.id, src: img.src } }));
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const label = varAttrs.map((a) => attrValue(a) || t("admin.variations.any")).join(" / ");
  return (
    <div className="variation card-inner">
      <div className="variation-head">
        <button type="button" className="link-like" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
          {open ? <FiChevronDown aria-hidden="true" /> : <FiChevronRight aria-hidden="true" className="flip-rtl" />} <strong>#{v.id}</strong> {label}
          <span className="muted small">
            {" "}
            {v.regularPrice !== "" && v.regularPrice !== null && v.regularPrice !== undefined ? money(v.salePrice || v.regularPrice) : t("admin.variations.noPrice")} · {t(`stock.${v.stockStatus || "instock"}`)}
          </span>
        </button>
        <button type="button" className="icon-btn danger" onClick={() => onDelete(v)} aria-label={t("common.delete")}>
          <FiTrash2 />
        </button>
      </div>
      <div className="row wrap">
        {varAttrs.map((a) => (
          <SelectField
            key={a.slug || a.name}
            label={a.name}
            value={attrValue(a)}
            onChange={(o) => setAttr(a, o)}
            placeholder={t("admin.variations.anyOption", { name: a.name })}
            options={(a.options || []).map((o) => ({ value: o, label: o }))}
          />
        ))}
      </div>
      {open && (
        <div className="stack mt">
          <div className="variation-image">
            {v.image?.src ? <img src={v.image.src} alt="" width="72" height="72" className="thumb-md" /> : <span className="img-placeholder thumb-md" />}
            <div className="stack">
              <UploadButton label={t("admin.products.uploadImage")} onFiles={upload} />
              {(product.images || []).length > 0 && (
                <SelectField
                  label={t("admin.variations.fromGallery")}
                  value={v.image?.id || ""}
                  onChange={(id) => {
                    const img = product.images.find((i) => String(i.id) === String(id));
                    setV({ ...v, image: img ? { id: img.id, src: img.src } : null });
                  }}
                  placeholder={t("admin.variations.noImage")}
                  options={product.images.map((i) => ({ value: i.id, label: i.name || i.src.split("/").pop() }))}
                />
              )}
            </div>
          </div>
          <div className="row wrap">
            <Toggle checked={v.status !== "private"} onChange={(on) => set("status")(on ? "publish" : "private")} label={t("admin.variations.enabled")} />
            <Toggle checked={!!v.downloadable} onChange={set("downloadable")} label={t("admin.products.downloadable")} />
            <Toggle checked={!!v.virtual} onChange={set("virtual")} label={t("admin.products.virtual")} />
            <Toggle checked={!!v.manageStock} onChange={set("manageStock")} label={t("admin.products.manageStock")} />
          </div>
          <div className="form-grid-3">
            <TextField label={t("product.sku")} value={v.sku} onChange={set("sku")} />
            <TextField label={t("admin.products.regularPrice")} type="number" step="0.01" min="0" value={v.regularPrice ?? ""} onChange={set("regularPrice")} />
            <TextField label={t("admin.products.salePrice")} type="number" step="0.01" min="0" value={v.salePrice ?? ""} onChange={set("salePrice")} />
            <TextField label={t("admin.products.saleFrom")} type="date" value={dateInput(v.dateOnSaleFrom)} onChange={(d) => set("dateOnSaleFrom")(dateToApi(d))} />
            <TextField label={t("admin.products.saleTo")} type="date" value={dateInput(v.dateOnSaleTo)} onChange={(d) => set("dateOnSaleTo")(dateToApi(d))} />
            {v.manageStock ? (
              <TextField label={t("admin.products.stockQuantity")} type="number" step="1" value={v.stockQuantity ?? 0} onChange={set("stockQuantity")} />
            ) : (
              <SelectField label={t("admin.products.stockStatus")} value={v.stockStatus || "instock"} onChange={set("stockStatus")} options={["instock", "outofstock", "onbackorder"].map((s) => ({ value: s, label: t(`stock.${s}`) }))} />
            )}
            {v.manageStock && (
              <SelectField label={t("admin.products.backorders")} value={v.backorders || "no"} onChange={set("backorders")} options={["no", "notify", "yes"].map((s) => ({ value: s, label: t(`backorders.${s}`) }))} />
            )}
            <TextField label={t("admin.products.weight")} value={v.weight} onChange={set("weight")} />
            <TextField label={t("admin.products.length")} value={v.length} onChange={set("length")} />
            <TextField label={t("admin.products.width")} value={v.width} onChange={set("width")} />
            <TextField label={t("admin.products.height")} value={v.height} onChange={set("height")} />
            <SelectField
              label={t("admin.products.shippingClass")}
              value={v.shippingClass || ""}
              onChange={set("shippingClass")}
              placeholder={t("admin.variations.sameAsParent")}
              options={shippingClasses.map((c) => ({ value: c.slug, label: c.name }))}
            />
            <SelectField
              label={t("admin.products.taxClass")}
              value={v.taxClass ?? "parent"}
              onChange={set("taxClass")}
              options={[{ value: "parent", label: t("admin.variations.sameAsParent") }, { value: "", label: t("tax.standard") }, ...taxClasses.map((c) => ({ value: c.slug, label: c.name }))]}
            />
          </div>
          <TextArea label={t("admin.products.description")} value={v.description} onChange={set("description")} rows={2} />
          <div>
            <button type="button" className="btn btn-primary btn-sm" onClick={save} disabled={busy}>
              {t("admin.variations.save")}
            </button>
          </div>
        </div>
      )}
      {!open && (
        <div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={save} disabled={busy}>
            {t("admin.variations.save")}
          </button>
        </div>
      )}
    </div>
  );
};

const ProductVariations = ({ product, shippingClasses, taxClasses }) => {
  const { t, errMsg } = useI18n();
  const { data: variations, loading, error, reload, setData } = useAsync(async () => asArray((await api.get(`/v1/admin/products/${product.id}/variations`)).data), [product.id], []);
  const [deleting, setDeleting] = useState(null);
  const [busy, setBusy] = useState(false);

  const run = async (fn, okMsg) => {
    setBusy(true);
    try {
      await fn();
      if (okMsg) toast.success(okMsg);
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  if (!product.id) return <p className="muted">{t("admin.variations.saveFirst")}</p>;
  const hasVarAttrs = (product.attributes || []).some((a) => a.variation && (a.options || []).length);

  return (
    <div className="stack">
      {!hasVarAttrs && <p className="notice notice-info">{t("admin.variations.needAttributes")}</p>}
      <div className="row wrap">
        <button type="button" className="btn btn-secondary btn-sm" disabled={busy || !hasVarAttrs} onClick={() => run(() => api.post(`/v1/admin/products/${product.id}/variations/generate`, {}), t("admin.variations.generated"))}>
          {t("admin.variations.generate")}
        </button>
        <button type="button" className="btn btn-secondary btn-sm" disabled={busy || !hasVarAttrs} onClick={() => run(() => api.post(`/v1/admin/products/${product.id}/variations`, { attributes: [] }))}>
          {t("admin.variations.add")}
        </button>
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : variations.length === 0 ? (
        <p className="muted small">{t("admin.variations.none")}</p>
      ) : (
        variations.map((v) => (
          <VariationEditor
            key={v.id}
            product={product}
            variation={v}
            shippingClasses={shippingClasses}
            taxClasses={taxClasses}
            onSaved={(saved) => setData((list) => list.map((x) => (x.id === saved.id ? saved : x)))}
            onDelete={setDeleting}
          />
        ))
      )}
      <ConfirmModal
        show={!!deleting}
        title={t("admin.variations.deleteTitle")}
        body={t("admin.variations.deleteBody")}
        confirmLabel={t("common.delete")}
        onConfirm={() => {
          const v = deleting;
          setDeleting(null);
          run(() => api.delete(`/v1/admin/products/${product.id}/variations/${v.id}`));
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default ProductVariations;
