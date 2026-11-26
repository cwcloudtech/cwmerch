import { useEffect, useMemo, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";
import { api, asArray, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useSeo } from "../../lib/seo";
import { stripHtml } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { ProductGrid } from "../../components/ProductCard";
import Breadcrumbs from "../../components/Breadcrumbs";
import { Empty, ErrorBox, Pagination, Spinner, Toggle } from "../../components/ui";

const PER_PAGE = 12;
const SORTS = ["menu_order", "popularity", "rating", "date", "price", "price-desc"];

const CategoryTree = ({ categories, active, parent = 0, depth = 0 }) => {
  const children = categories.filter((c) => Number(c.parent || 0) === Number(parent));
  if (children.length === 0) return null;
  return (
    <ul className={depth ? "cat-tree-sub" : "cat-tree"}>
      {children.map((c) => (
        <li key={c.id}>
          <Link to={`/product-category/${c.slug}`} className={active === c.slug ? "is-active" : ""} aria-current={active === c.slug ? "page" : undefined}>
            {c.name} <small className="muted">({c.count || 0})</small>
          </Link>
          <CategoryTree categories={categories} active={active} parent={c.id} depth={depth + 1} />
        </li>
      ))}
    </ul>
  );
};

const Catalogue = ({ mode = "shop" }) => {
  const { slug } = useParams();
  const [params, setParams] = useSearchParams();
  const { categories, shopName, money } = useShop();
  const { t } = useI18n();
  const [filtersOpen, setFiltersOpen] = useState(false);
  const { data: tags } = useAsync(async () => asArray((await api.get("/v1/shop/tags")).data), [], []);

  const page = Math.max(1, Number(params.get("page")) || 1);
  const search = params.get("search") || "";
  const orderby = params.get("orderby") || "menu_order";
  const onSale = params.get("onSale") === "1";
  const minPrice = params.get("minPrice") || "";
  const maxPrice = params.get("maxPrice") || "";
  const category = mode === "category" ? slug : params.get("category") || "";
  const tag = mode === "tag" ? slug : params.get("tag") || "";

  const [priceDraft, setPriceDraft] = useState({ min: minPrice, max: maxPrice });
  useEffect(() => setPriceDraft({ min: minPrice, max: maxPrice }), [minPrice, maxPrice]);

  const currentCategory = categories.find((c) => c.slug === category);
  const currentTag = tags.find((x) => x.slug === tag);

  const { data, loading, error } = useAsync(
    async () =>
      normalizePaged(
        (
          await api.get("/v1/shop/products", {
            params: {
              search: search || undefined,
              category: category || undefined,
              tag: tag || undefined,
              minPrice: minPrice || undefined,
              maxPrice: maxPrice || undefined,
              onSale: onSale || undefined,
              orderby,
              page,
              perPage: PER_PAGE,
            },
          })
        ).data,
        page,
        PER_PAGE
      ),
    [search, category, tag, minPrice, maxPrice, onSale, orderby, page],
    { items: [], total: 0, page: 1, totalPages: 1 }
  );

  const update = (changes) => {
    const next = new URLSearchParams(params);
    Object.entries(changes).forEach(([k, v]) => {
      if (v === "" || v === null || v === undefined || v === false) next.delete(k);
      else next.set(k, v === true ? "1" : String(v));
    });
    if (!("page" in changes)) next.delete("page");
    setParams(next);
  };

  const title = currentCategory ? currentCategory.name : currentTag ? currentTag.name : search ? t("shop.resultsFor", { search }) : t("shop.title");
  const crumbs = useMemo(() => {
    const list = [{ label: t("nav.shop"), to: "/shop" }];
    if (currentCategory) {
      const chain = [];
      let c = currentCategory;
      while (c) {
        chain.unshift(c);
        const parentId = Number(c.parent || 0);
        c = parentId ? categories.find((x) => Number(x.id) === parentId) : null;
      }
      chain.forEach((x) => list.push({ label: x.name, to: `/product-category/${x.slug}` }));
    } else if (currentTag) list.push({ label: currentTag.name });
    else if (search) list.push({ label: title });
    return list;
  }, [currentCategory, currentTag, categories, search, title, t]);

  useSeo({
    title: `${title} – ${shopName}`,
    description: stripHtml(currentCategory?.description || currentTag?.description || "") || t("shop.seoDescription", { shop: shopName }),
    image: currentCategory?.image?.src,
    noindex: !!search,
    jsonLd: {
      "@context": "https://schema.org",
      "@type": "BreadcrumbList",
      itemListElement: crumbs.map((c, i) => ({ "@type": "ListItem", position: i + 1, name: c.label, ...(c.to ? { item: window.location.origin + c.to } : {}) })),
    },
  });

  return (
    <div className="container catalogue">
      <Breadcrumbs items={crumbs} />
      <header className="page-header">
        <h1>{title}</h1>
        {currentCategory?.description && <div className="term-description" dangerouslySetInnerHTML={{ __html: currentCategory.description }} />}
      </header>
      <div className="catalogue-layout">
        <aside className={`filters ${filtersOpen ? "is-open" : ""}`} aria-label={t("shop.filters")}>
          <section>
            <h2 className="filter-title">{t("shop.categories")}</h2>
            <ul className="cat-tree">
              <li>
                <Link to="/shop" className={!category && mode === "shop" ? "is-active" : ""}>
                  {t("shop.allProducts")}
                </Link>
              </li>
            </ul>
            <CategoryTree categories={categories} active={category} />
          </section>
          <section>
            <h2 className="filter-title">{t("shop.price")}</h2>
            <form
              className="price-filter"
              onSubmit={(e) => {
                e.preventDefault();
                update({ minPrice: priceDraft.min, maxPrice: priceDraft.max });
              }}
            >
              <label>
                <span className="sr-only">{t("shop.minPrice")}</span>
                <input className="input input-sm" type="number" min="0" step="any" placeholder={t("shop.min")} value={priceDraft.min} onChange={(e) => setPriceDraft({ ...priceDraft, min: e.target.value })} />
              </label>
              <span aria-hidden="true">–</span>
              <label>
                <span className="sr-only">{t("shop.maxPrice")}</span>
                <input className="input input-sm" type="number" min="0" step="any" placeholder={t("shop.max")} value={priceDraft.max} onChange={(e) => setPriceDraft({ ...priceDraft, max: e.target.value })} />
              </label>
              <button type="submit" className="btn btn-secondary btn-sm">
                {t("shop.filter")}
              </button>
            </form>
            {(minPrice || maxPrice) && (
              <p className="small muted">
                {minPrice ? money(minPrice) : "…"} – {maxPrice ? money(maxPrice) : "…"}
              </p>
            )}
          </section>
          <section>
            <Toggle checked={onSale} onChange={(v) => update({ onSale: v })} label={t("shop.onSaleOnly")} />
          </section>
          {tags.length > 0 && (
            <section>
              <h2 className="filter-title">{t("shop.tags")}</h2>
              <div className="tag-cloud">
                {tags.map((x) => (
                  <Link key={x.id} to={`/product-tag/${x.slug}`} className={`chip ${tag === x.slug ? "is-active" : ""}`}>
                    {x.name}
                  </Link>
                ))}
              </div>
            </section>
          )}
        </aside>
        <div className="catalogue-main">
          <div className="catalogue-toolbar">
            <button type="button" className="btn btn-secondary btn-sm show-mobile" aria-expanded={filtersOpen} onClick={() => setFiltersOpen((o) => !o)}>
              {t("shop.filters")}
            </button>
            <p className="muted small" aria-live="polite">
              {t("shop.resultCount", { count: data.total })}
            </p>
            <label className="inline-field">
              <span>{t("shop.sortBy")}</span>
              <select className="input input-sm" value={orderby} onChange={(e) => update({ orderby: e.target.value === "menu_order" ? "" : e.target.value })}>
                {SORTS.map((s) => (
                  <option key={s} value={s}>
                    {t(`shop.sort.${s}`)}
                  </option>
                ))}
              </select>
            </label>
          </div>
          <ErrorBox error={error} />
          {loading ? (
            <Spinner />
          ) : data.items.length === 0 ? (
            <Empty title={t("shop.noProducts")} body={t("shop.noProductsBody")}>
              <Link to="/shop" className="btn btn-secondary">
                {t("shop.allProducts")}
              </Link>
            </Empty>
          ) : (
            <ProductGrid products={data.items} />
          )}
          <Pagination page={data.page || page} totalPages={data.totalPages} onChange={(p) => update({ page: p })} />
        </div>
      </div>
    </div>
  );
};

export default Catalogue;
