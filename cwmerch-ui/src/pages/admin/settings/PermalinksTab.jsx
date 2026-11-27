import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../../api/client";
import { useShop } from "../../../context/ShopContext";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Spinner, TextField } from "../../../components/ui";

// PermalinksTab edits WooCommerce's URL bases (GET/PUT /v1/admin/permalinks,
// the woocommerce_permalinks option WordPress reads too), so product,
// category and tag pages keep the same addresses on both.
const PRESETS = [
  { key: "shopBase", product: (pl) => pl.shopBase },
  { key: "shopCategory", product: (pl) => `${pl.shopBase}/%product_cat%` },
];

const PermalinksTab = () => {
  const { t, errMsg } = useI18n();
  const { reload: reloadShop } = useShop();
  const [pl, setPl] = useState(null);
  const [form, setForm] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .get("/v1/admin/permalinks")
      .then(({ data }) => {
        setPl(data);
        setForm({ productBase: data.productBase, categoryBase: data.categoryBase, tagBase: data.tagBase, attributeBase: data.attributeBase || "" });
      })
      .catch(setError);
  }, []);

  if (!form) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));
  const example = (base) => `/${String(base || "").replace(/^\/+|\/+$/g, "").replace("%product_cat%", "clothing")}/sample-product`;

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const { data } = await api.put("/v1/admin/permalinks", form);
      setPl(data);
      setForm({ productBase: data.productBase, categoryBase: data.categoryBase, tagBase: data.tagBase, attributeBase: data.attributeBase || "" });
      toast.success(t("admin.saved"));
      reloadShop();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="stack-lg" onSubmit={save}>
      <section className="card stack settings-section">
        <h2>{t("permalinks.title")}</h2>
        <p className="muted small">{t("permalinks.intro")}</p>
        <p className="muted small">{t("permalinks.shopPage", { path: `/${pl.shopBase}` })}</p>
        <div className="chip-row">
          {PRESETS.map((p) => (
            <button
              key={p.key}
              type="button"
              className="btn btn-secondary btn-sm"
              onClick={() => set("productBase")(p.product(pl))}
            >
              {t(`permalinks.presets.${p.key}`)}
            </button>
          ))}
        </div>
        <TextField label={t("permalinks.productBase")} dir="ltr" hint={t("permalinks.productBaseHint", { example: example(form.productBase) })} value={form.productBase} onChange={set("productBase")} required />
        <TextField label={t("permalinks.categoryBase")} dir="ltr" hint={`/${form.categoryBase}/clothing/hoodies`} value={form.categoryBase} onChange={set("categoryBase")} required />
        <TextField label={t("permalinks.tagBase")} dir="ltr" hint={`/${form.tagBase}/summer`} value={form.tagBase} onChange={set("tagBase")} required />
        <TextField label={t("permalinks.attributeBase")} dir="ltr" value={form.attributeBase} onChange={set("attributeBase")} />
        <p className="notice notice-info small">{t("permalinks.wordpressNotice")}</p>
      </section>
      <div className="sticky-save">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

export default PermalinksTab;
