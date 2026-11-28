import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiCopy, FiEdit2, FiStar, FiTrash2 } from "react-icons/fi";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { ConfirmModal, ErrorBox, Pagination, Spinner, Tabs } from "../../components/ui";
import { ScoreDot } from "../../components/seo/SeoPanel";

// The stored score (0-100) as a traffic light (no keyphrase: not analysed).
const ratingOf = (score, keyphrase) => (score === undefined || score === null || score < 0 || !keyphrase ? "na" : score > 70 ? "good" : score > 40 ? "ok" : "bad");

const STATUSES = ["", "publish", "draft", "pending", "private", "trash"];

const Products = () => {
  const { t } = useI18n();
  const { money, categories } = useShop();
  const navigate = useNavigate();
  const [params, setParams] = useSearchParams();
  const q = Object.fromEntries(params.entries());
  const page = Number(q.page) || 1;
  const [deleting, setDeleting] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () =>
      normalizePaged(
        (
          await api.get("/v1/admin/products", {
            params: {
              search: q.search || undefined,
              status: q.status || undefined,
              type: q.type || undefined,
              category: q.category || undefined,
              stockStatus: q.stockStatus || undefined,
              page,
              perPage: 20,
            },
          })
        ).data,
        page
      ),
    [params.toString()],
    { items: [], totalPages: 1 }
  );
  const update = (changes) => {
    const next = new URLSearchParams(params);
    Object.entries(changes).forEach(([k, v]) => (v ? next.set(k, v) : next.delete(k)));
    if (!("page" in changes)) next.delete("page");
    setParams(next);
  };

  const duplicate = async (p) => {
    try {
      const { data: copy } = await api.post(`/v1/admin/products/${p.id}/duplicate`, {});
      toast.success(t("admin.products.duplicated"));
      if (copy?.id) navigate(`/admin/products/${copy.id}`);
      else reload();
    } catch (err) {
      toast.error(err?.response?.data?.message || t("errors.generic"));
    }
  };
  const remove = async () => {
    const { product, force } = deleting;
    setDeleting(null);
    try {
      await api.delete(`/v1/admin/products/${product.id}`, { params: force ? { force: true } : {} });
      toast.success(force ? t("admin.deleted") : t("admin.trashed"));
      reload();
    } catch (err) {
      toast.error(err?.response?.data?.message || t("errors.generic"));
    }
  };
  const toggleFeatured = async (p) => {
    try {
      await api.put(`/v1/admin/products/${p.id}`, { featured: !p.featured });
      reload();
    } catch {
      toast.error(t("errors.generic"));
    }
  };

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.products")}>
        <Link to="/admin/products/new" className="btn btn-primary btn-sm">
          {t("admin.nav.addProduct")}
        </Link>
      </PageHead>
      <Tabs tabs={STATUSES.map((s) => ({ id: s, label: s ? t(`productStatus.${s}`) : t("common.all") }))} active={q.status || ""} onChange={(s) => update({ status: s })} />
      <div className="toolbar">
        <div className="row wrap">
          <label className="sr-only" htmlFor="f-cat">
            {t("product.categories")}
          </label>
          <select id="f-cat" className="input input-sm" value={q.category || ""} onChange={(e) => update({ category: e.target.value })}>
            <option value="">{t("admin.products.anyCategory")}</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
          <label className="sr-only" htmlFor="f-type">
            {t("admin.products.type")}
          </label>
          <select id="f-type" className="input input-sm" value={q.type || ""} onChange={(e) => update({ type: e.target.value })}>
            <option value="">{t("admin.products.anyType")}</option>
            {["simple", "variable", "grouped", "external"].map((x) => (
              <option key={x} value={x}>
                {t(`productType.${x}`)}
              </option>
            ))}
          </select>
          <label className="sr-only" htmlFor="f-stock">
            {t("admin.products.stock")}
          </label>
          <select id="f-stock" className="input input-sm" value={q.stockStatus || ""} onChange={(e) => update({ stockStatus: e.target.value })}>
            <option value="">{t("admin.products.anyStock")}</option>
            {["instock", "outofstock", "onbackorder"].map((x) => (
              <option key={x} value={x}>
                {t(`stock.${x}`)}
              </option>
            ))}
          </select>
        </div>
        <SearchBox value={q.search} onSearch={(s) => update({ search: s })} placeholder={t("admin.products.search")} />
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table">
            <thead>
              <tr>
                <th scope="col">
                  <span className="sr-only">{t("admin.products.image")}</span>
                </th>
                <th scope="col">{t("admin.products.name")}</th>
                <th scope="col">{t("product.sku")}</th>
                <th scope="col">{t("admin.products.stock")}</th>
                <th scope="col" className="num">
                  {t("cart.price")}
                </th>
                <th scope="col">{t("product.categories")}</th>
                <th scope="col" className="center">
                  <abbr title={t("seo.seoAnalysis")}>SEO</abbr>
                </th>
                <th scope="col" className="center">
                  <abbr title={t("seo.readability")}>{t("seo.readabilityShort")}</abbr>
                </th>
                <th scope="col">
                  <span className="sr-only">{t("admin.products.featured")}</span>
                </th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((p) => (
                <tr key={p.id}>
                  <td>{p.images?.[0] ? <img src={p.images[0].src} alt="" width="40" height="40" className="thumb-sm" /> : <span className="img-placeholder thumb-sm" />}</td>
                  <td>
                    <Link to={`/admin/products/${p.id}`}>
                      <strong>{p.name || t("admin.products.untitled")}</strong>
                    </Link>
                    {p.status !== "publish" && <span className="muted small"> — {t(`productStatus.${p.status}`)}</span>}
                    <div className="small muted">
                      {t(`productType.${p.type}`)} · ID {p.id}
                    </div>
                  </td>
                  <td>{p.sku || "—"}</td>
                  <td>
                    <span className={`stock ${p.stockStatus}`}>{t(`stock.${p.stockStatus}`)}</span>
                    {p.manageStock && <span className="muted small"> ({p.stockQuantity})</span>}
                  </td>
                  <td className="num">
                    {p.onSale && Number(p.regularPrice) > Number(p.price) && <del className="muted small">{money(p.regularPrice)} </del>}
                    {p.type === "variable" && Number(p.priceMin) !== Number(p.priceMax) ? `${money(p.priceMin)} – ${money(p.priceMax)}` : p.price !== "" && p.price !== null ? money(p.price) : "—"}
                  </td>
                  <td className="small">{(p.categories || []).map((c) => c.name).join(", ") || "—"}</td>
                  <td className="center">
                    <ScoreDot rating={ratingOf(p.seoScore, p.focusKeyphrase)} title={`${t("seo.seoAnalysis")}: ${t(`seo.ratings.${ratingOf(p.seoScore, p.focusKeyphrase)}`)}`} />
                  </td>
                  <td className="center">
                    <ScoreDot rating={ratingOf(p.readabilityScore, "x")} title={`${t("seo.readability")}: ${t(`seo.ratings.${ratingOf(p.readabilityScore, "x")}`)}`} />
                  </td>
                  <td>
                    <button type="button" className={`icon-btn ${p.featured ? "is-on" : ""}`} onClick={() => toggleFeatured(p)} aria-pressed={!!p.featured} aria-label={t("admin.products.toggleFeatured", { name: p.name })}>
                      <FiStar />
                    </button>
                  </td>
                  <td className="row-actions">
                    <Link to={`/admin/products/${p.id}`} className="icon-btn" aria-label={t("common.edit")}>
                      <FiEdit2 />
                    </Link>
                    <button type="button" className="icon-btn" onClick={() => duplicate(p)} aria-label={t("admin.products.duplicate")}>
                      <FiCopy />
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => setDeleting({ product: p, force: p.status === "trash" })} aria-label={p.status === "trash" ? t("admin.deletePermanently") : t("admin.trash")}>
                      <FiTrash2 />
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("shop.noProducts")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={(p) => update({ page: String(p) })} />
      <ConfirmModal
        show={!!deleting}
        title={deleting?.force ? t("admin.deletePermanently") : t("admin.trash")}
        body={deleting ? t(deleting.force ? "admin.products.deleteBody" : "admin.products.trashBody", { name: deleting.product.name }) : ""}
        confirmLabel={deleting?.force ? t("admin.deletePermanently") : t("admin.trash")}
        onConfirm={remove}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default Products;
