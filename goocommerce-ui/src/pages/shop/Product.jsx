import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiShoppingBag, FiExternalLink } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useSeo, truncate } from "../../lib/seo";
import { sameOption } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useCart } from "../../context/CartContext";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import Price from "../../components/Price";
import { ProductGrid } from "../../components/ProductCard";
import Breadcrumbs from "../../components/Breadcrumbs";
import { Empty, ErrorBox, Spinner, StarInput, Stars, Tabs, TextArea, TextField } from "../../components/ui";

const attrKey = (a) => a.slug || a.name;

// matchVariation resolves the variation for the selected attributes: an empty
// option on the variation means "any".
const matchVariation = (variations, attributes, selected) => {
  const varAttrs = attributes.filter((a) => a.variation);
  if (varAttrs.some((a) => !selected[attrKey(a)])) return null;
  return (
    variations.find((v) =>
      (v.attributes || []).every((va) => {
        const key = va.slug || va.name;
        const sel = selected[key] ?? selected[varAttrs.find((a) => sameOption(a.name, va.name))?.slug];
        return !va.option || (sel && sameOption(va.option, sel));
      })
    ) || null
  );
};

const stockText = (item, t) => {
  if (!item) return "";
  if (item.stockStatus === "outofstock") return t("product.outOfStock");
  if (item.stockStatus === "onbackorder") return t("product.onBackorder");
  if (item.manageStock && item.stockQuantity !== null && item.stockQuantity !== undefined) return t("product.inStockCount", { count: item.stockQuantity });
  return t("product.inStock");
};

const QuantityInput = ({ value, onChange, max, id }) => {
  const { t } = useI18n();
  return (
    <span className="qty">
      <label htmlFor={id} className="sr-only">
        {t("cart.quantity")}
      </label>
      <input id={id} className="input qty-input" type="number" min="1" max={max || undefined} step="1" value={value} onChange={(e) => onChange(Math.max(1, Math.min(max || Infinity, Number(e.target.value) || 1)))} />
    </span>
  );
};

const Gallery = ({ images, override, name }) => {
  const { t } = useI18n();
  const list = useMemo(() => {
    const base = images || [];
    if (override?.src && !base.some((i) => i.src === override.src)) return [override, ...base];
    return base;
  }, [images, override]);
  const [active, setActive] = useState(0);
  useEffect(() => {
    if (override?.src) {
      const i = list.findIndex((x) => x.src === override.src);
      if (i >= 0) setActive(i);
    }
  }, [override, list]);
  if (list.length === 0) return <div className="gallery"><div className="img-placeholder gallery-main" aria-hidden="true" /></div>;
  const main = list[Math.min(active, list.length - 1)];
  return (
    <div className="gallery">
      <div className="gallery-main">
        <img src={main.src} alt={main.alt || name} />
      </div>
      {list.length > 1 && (
        <div className="gallery-thumbs" role="list">
          {list.map((img, i) => (
            <button key={img.id || img.src} type="button" role="listitem" className={`thumb ${i === active ? "is-active" : ""}`} onClick={() => setActive(i)} aria-label={t("product.showImage", { n: i + 1 })} aria-pressed={i === active}>
              <img src={img.src} alt="" loading="lazy" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
};

const Reviews = ({ product, onPosted }) => {
  const { t, errMsg, formatDate } = useI18n();
  const { settings } = useShop();
  const { user } = useAuth();
  const { data: reviews, loading, reload } = useAsync(async () => asArray((await api.get(`/v1/shop/products/${product.id}/reviews`)).data), [product.id], []);
  const [form, setForm] = useState({ rating: 0, review: "", author: "", email: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const ratingOn = settings.enableReviewRating !== false;
  const ratingRequired = ratingOn && settings.reviewRatingRequired;
  const canReview = settings.enableReviews !== false && product.reviewsAllowed !== false;

  const submit = async (e) => {
    e.preventDefault();
    setError("");
    if (ratingRequired && !form.rating) {
      setError(t("reviews.ratingRequired"));
      return;
    }
    setBusy(true);
    try {
      await api.post(`/v1/shop/products/${product.id}/reviews`, {
        rating: form.rating || 0,
        review: form.review,
        ...(user ? {} : { author: form.author, email: form.email }),
      });
      toast.success(t("reviews.thanks"));
      setForm({ rating: 0, review: "", author: "", email: "" });
      reload();
      onPosted?.();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="reviews">
      <h2>{reviews.length ? t("reviews.countFor", { count: reviews.length, name: product.name }) : t("reviews.title")}</h2>
      {loading ? (
        <Spinner />
      ) : reviews.length === 0 ? (
        <p className="muted">{t("reviews.none")}</p>
      ) : (
        <ol className="review-list">
          {reviews.map((r) => (
            <li key={r.id} className="review">
              <div className="review-head">
                <strong>{r.reviewer}</strong>
                {r.verified && <span className="badge badge-completed">{t("reviews.verified")}</span>}
                <time className="muted small" dateTime={r.createdAt}>
                  {formatDate(r.createdAt)}
                </time>
              </div>
              {ratingOn && Number(r.rating) > 0 && <Stars rating={r.rating} />}
              <p>{r.review}</p>
            </li>
          ))}
        </ol>
      )}
      {canReview && (
        <form className="review-form card" onSubmit={submit}>
          <h3>{reviews.length ? t("reviews.add") : t("reviews.beFirst", { name: product.name })}</h3>
          {settings.verifiedOwnersOnly && <p className="muted small">{t("reviews.verifiedOnly")}</p>}
          {ratingOn && <StarInput value={form.rating} onChange={(rating) => setForm({ ...form, rating })} required={ratingRequired} />}
          <TextArea label={t("reviews.yourReview")} value={form.review} onChange={(review) => setForm({ ...form, review })} required rows={4} />
          {!user && (
            <div className="form-grid">
              <TextField label={t("reviews.name")} value={form.author} onChange={(author) => setForm({ ...form, author })} required autoComplete="name" />
              <TextField label={t("reviews.email")} type="email" value={form.email} onChange={(email) => setForm({ ...form, email })} required autoComplete="email" />
            </div>
          )}
          <ErrorBox error={error} />
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("reviews.submit")}
          </button>
        </form>
      )}
    </div>
  );
};

const GroupedChildren = ({ ids }) => {
  const { t } = useI18n();
  const { addItem } = useCart();
  const { data: children, loading } = useAsync(
    async () => (await Promise.allSettled(ids.map((id) => api.get(`/v1/shop/products/${id}`)))).filter((r) => r.status === "fulfilled").map((r) => r.value.data),
    [ids.join(",")],
    []
  );
  const [qty, setQty] = useState({});
  if (loading) return <Spinner />;
  const addAll = () => {
    let n = 0;
    children.forEach((c) => {
      const q = Number(qty[c.id] || 0);
      if (q > 0) {
        addItem(c.id, 0, q, c.soldIndividually);
        n += q;
      }
    });
    if (n) toast.success(t("cart.addedCount", { count: n }));
  };
  return (
    <div className="grouped">
      <table className="table">
        <tbody>
          {children.map((c) => (
            <tr key={c.id}>
              <td>
                {c.type === "simple" && c.stockStatus !== "outofstock" ? (
                  <input
                    className="input qty-input"
                    type="number"
                    min="0"
                    max={c.manageStock && c.backorders === "no" ? c.stockQuantity : undefined}
                    value={qty[c.id] || 0}
                    aria-label={t("product.quantityFor", { name: c.name })}
                    onChange={(e) => setQty({ ...qty, [c.id]: Math.max(0, Number(e.target.value) || 0) })}
                  />
                ) : (
                  <Link to={`/product/${c.slug}`} className="btn btn-secondary btn-sm">
                    {t("product.viewProduct")}
                  </Link>
                )}
              </td>
              <td>
                <Link to={`/product/${c.slug}`}>{c.name}</Link>
              </td>
              <td>
                <Price item={c} />
              </td>
              <td className="small muted">{stockText(c, t)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <button type="button" className="btn btn-primary" onClick={addAll}>
        <FiShoppingBag aria-hidden="true" /> {t("product.addToCart")}
      </button>
    </div>
  );
};

const Related = ({ ids }) => {
  const { t } = useI18n();
  const { data } = useAsync(
    async () => (await Promise.allSettled(ids.slice(0, 4).map((id) => api.get(`/v1/shop/products/${id}`)))).filter((r) => r.status === "fulfilled").map((r) => r.value.data),
    [ids.join(",")],
    []
  );
  if (data.length === 0) return null;
  return (
    <section className="related">
      <h2>{t("product.related")}</h2>
      <ProductGrid products={data} />
    </section>
  );
};

const Product = () => {
  const { slug } = useParams();
  const { t } = useI18n();
  const { settings, shopName, categories } = useShop();
  const { addItem } = useCart();
  const { data: product, loading, error, reload } = useAsync(async () => (await api.get(`/v1/shop/products/${encodeURIComponent(slug)}`)).data, [slug]);
  const [selected, setSelected] = useState({});
  const [quantity, setQuantity] = useState(1);
  const [tab, setTab] = useState("description");

  useEffect(() => {
    if (!product) return;
    const defaults = {};
    (product.defaultAttributes || []).forEach((d) => {
      defaults[d.slug || d.name] = d.option;
    });
    setSelected(defaults);
    setQuantity(1);
    setTab(product.description ? "description" : "additional");
  }, [product]);

  const variation = useMemo(
    () => (product?.type === "variable" ? matchVariation(product.variations || [], product.attributes || [], selected) : null),
    [product, selected]
  );

  const item = product?.type === "variable" ? variation : product;
  const purchasable = item && item.stockStatus !== "outofstock" && (product?.type !== "variable" || variation) && item.price !== "" && item.price !== null;
  const maxQty = item && item.manageStock && item.backorders === "no" && Number(item.stockQuantity) > 0 ? Number(item.stockQuantity) : undefined;

  const firstImage = product?.images?.[0]?.src;
  const crumbCat = product?.categories?.[0];
  const fullCat = crumbCat ? categories.find((c) => c.id === crumbCat.id) || crumbCat : null;
  const availability = product?.stockStatus === "outofstock" ? "OutOfStock" : product?.stockStatus === "onbackorder" ? "BackOrder" : "InStock";

  useSeo({
    title: product ? `${product.name} – ${shopName}` : shopName,
    description: product ? truncate(product.shortDescription || product.description, 160) : "",
    image: firstImage,
    type: "product",
    jsonLd: product
      ? [
          {
            "@context": "https://schema.org",
            "@type": "Product",
            name: product.name,
            description: truncate(product.shortDescription || product.description, 300),
            sku: product.sku || undefined,
            image: (product.images || []).map((i) => i.src),
            url: window.location.origin + `/product/${product.slug}`,
            offers:
              product.type === "variable" && Number(product.priceMin) !== Number(product.priceMax)
                ? { "@type": "AggregateOffer", lowPrice: product.priceMin, highPrice: product.priceMax, priceCurrency: settings.currency, availability: `https://schema.org/${availability}` }
                : { "@type": "Offer", price: product.price, priceCurrency: settings.currency, availability: `https://schema.org/${availability}`, url: window.location.origin + `/product/${product.slug}` },
            ...(Number(product.ratingCount) > 0
              ? { aggregateRating: { "@type": "AggregateRating", ratingValue: product.averageRating, reviewCount: product.ratingCount } }
              : {}),
          },
          {
            "@context": "https://schema.org",
            "@type": "BreadcrumbList",
            itemListElement: [
              { "@type": "ListItem", position: 1, name: t("nav.home"), item: window.location.origin + "/" },
              ...(fullCat ? [{ "@type": "ListItem", position: 2, name: fullCat.name, item: window.location.origin + `/product-category/${fullCat.slug}` }] : []),
              { "@type": "ListItem", position: fullCat ? 3 : 2, name: product.name },
            ],
          },
        ]
      : null,
  });

  if (loading) return <Spinner />;
  if (error || !product) {
    return (
      <div className="container">
        {error?.response?.status === 404 ? (
          <Empty title={t("product.notFound")}>
            <Link to="/shop" className="btn btn-primary">
              {t("shop.allProducts")}
            </Link>
          </Empty>
        ) : (
          <ErrorBox error={error} />
        )}
      </div>
    );
  }

  const add = () => {
    if (!purchasable) return;
    // A variation attribute left as "any" carries the option the customer picked.
    const anyAttrs = (variation?.attributes || [])
      .filter((va) => !va.option)
      .map((va) => ({ slug: va.slug || va.name, option: selected[va.slug || va.name] || "" }))
      .filter((a) => a.option);
    addItem(product.id, variation?.id || 0, product.soldIndividually ? 1 : quantity, product.soldIndividually, anyAttrs);
    toast.success(t("cart.added", { name: product.name }));
  };

  const varAttrs = (product.attributes || []).filter((a) => a.variation);
  const infoAttrs = (product.attributes || []).filter((a) => a.visible !== false);
  const hasDimensions = product.length || product.width || product.height;
  const showAdditional = infoAttrs.length > 0 || product.weight || hasDimensions;

  // Options available for an attribute given the other selections (WooCommerce greys out impossible combos).
  const optionAvailable = (attr, option) =>
    (product.variations || []).some((v) =>
      (v.attributes || []).every((va) => {
        const key = va.slug || va.name;
        const value = key === attrKey(attr) ? option : selected[key];
        return !va.option || !value || sameOption(va.option, value);
      })
    );

  const tabs = [
    ...(product.description ? [{ id: "description", label: t("product.description") }] : []),
    ...(showAdditional ? [{ id: "additional", label: t("product.additional") }] : []),
    ...(settings.enableReviews !== false && product.reviewsAllowed !== false ? [{ id: "reviews", label: t("product.reviewsTab", { count: product.ratingCount || 0 }) }] : []),
  ];

  return (
    <div className="container product-page">
      <Breadcrumbs items={[{ label: t("nav.shop"), to: "/shop" }, ...(fullCat ? [{ label: fullCat.name, to: `/product-category/${fullCat.slug}` }] : []), { label: product.name }]} />
      <div className="product-main">
        <Gallery images={product.images} override={variation?.image} name={product.name} />
        <div className="product-summary">
          <h1>{product.name}</h1>
          {Number(product.ratingCount) > 0 && (
            <button type="button" className="link-btn" onClick={() => setTab("reviews")}>
              <Stars rating={product.averageRating} count={product.ratingCount} />
            </button>
          )}
          <p className="product-price">
            <Price item={variation || product} range={!variation} />
          </p>
          {product.shortDescription && <div className="short-description" dangerouslySetInnerHTML={{ __html: product.shortDescription }} />}

          {product.type === "external" ? (
            <a className="btn btn-primary btn-lg" href={product.externalUrl} target="_blank" rel="noopener noreferrer">
              {product.buttonText || t("product.buy")} <FiExternalLink aria-hidden="true" />
            </a>
          ) : product.type === "grouped" ? (
            <GroupedChildren ids={product.groupedProducts || []} />
          ) : (
            <form
              className="add-to-cart"
              onSubmit={(e) => {
                e.preventDefault();
                add();
              }}
            >
              {product.type === "variable" && (
                <div className="variations">
                  {varAttrs.map((a) => (
                    <div className="field" key={attrKey(a)}>
                      <label className="field-label" htmlFor={`attr-${attrKey(a)}`}>
                        {a.name}
                      </label>
                      <select
                        id={`attr-${attrKey(a)}`}
                        className="input"
                        value={selected[attrKey(a)] || ""}
                        onChange={(e) => setSelected({ ...selected, [attrKey(a)]: e.target.value })}
                      >
                        <option value="">{t("product.chooseOption")}</option>
                        {(a.options || []).map((o) => (
                          <option key={o} value={o} disabled={!optionAvailable(a, o)}>
                            {o}
                          </option>
                        ))}
                      </select>
                    </div>
                  ))}
                  {Object.values(selected).some(Boolean) && (
                    <button type="button" className="link-btn small" onClick={() => setSelected({})}>
                      {t("product.clear")}
                    </button>
                  )}
                  {variation ? (
                    <div className="variation-info" aria-live="polite">
                      {variation.description && <div dangerouslySetInnerHTML={{ __html: variation.description }} />}
                      <p className={`stock ${variation.stockStatus}`}>{stockText(variation, t)}</p>
                      {variation.sku && <p className="small muted">{t("product.sku")}: {variation.sku}</p>}
                    </div>
                  ) : (
                    varAttrs.every((a) => selected[attrKey(a)]) && (
                      <p className="notice notice-error" role="status">
                        {t("product.noVariation")}
                      </p>
                    )
                  )}
                </div>
              )}
              {product.type !== "variable" && <p className={`stock ${product.stockStatus}`}>{stockText(product, t)}</p>}
              <div className="buy-row">
                {!product.soldIndividually && <QuantityInput id="qty" value={quantity} onChange={setQuantity} max={maxQty} />}
                <button type="submit" className="btn btn-primary btn-lg" disabled={!purchasable}>
                  <FiShoppingBag aria-hidden="true" /> {t("product.addToCart")}
                </button>
              </div>
            </form>
          )}

          <div className="product-meta small muted">
            {product.sku && (
              <p>
                {t("product.sku")}: {product.sku}
              </p>
            )}
            {product.categories?.length > 0 && (
              <p>
                {t("product.categories")}:{" "}
                {product.categories.map((c, i) => (
                  <span key={c.id}>
                    {i > 0 && ", "}
                    <Link to={`/product-category/${c.slug}`}>{c.name}</Link>
                  </span>
                ))}
              </p>
            )}
            {product.tags?.length > 0 && (
              <p>
                {t("product.tags")}:{" "}
                {product.tags.map((c, i) => (
                  <span key={c.id}>
                    {i > 0 && ", "}
                    <Link to={`/product-tag/${c.slug}`}>{c.name}</Link>
                  </span>
                ))}
              </p>
            )}
          </div>
        </div>
      </div>

      {tabs.length > 0 && (
        <div className="product-tabs">
          <Tabs tabs={tabs} active={tab} onChange={setTab} />
          <div className="tab-panel" role="tabpanel">
            {tab === "description" && <div className="description" dangerouslySetInnerHTML={{ __html: product.description }} />}
            {tab === "additional" && (
              <table className="table attributes-table">
                <tbody>
                  {product.weight && (
                    <tr>
                      <th scope="row">{t("product.weight")}</th>
                      <td>
                        {product.weight} {settings.weightUnit}
                      </td>
                    </tr>
                  )}
                  {hasDimensions && (
                    <tr>
                      <th scope="row">{t("product.dimensions")}</th>
                      <td>
                        {[product.length, product.width, product.height].filter(Boolean).join(" × ")} {settings.dimensionUnit}
                      </td>
                    </tr>
                  )}
                  {infoAttrs.map((a) => (
                    <tr key={attrKey(a)}>
                      <th scope="row">{a.name}</th>
                      <td>{(a.options || []).join(", ")}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            {tab === "reviews" && <Reviews product={product} onPosted={reload} />}
          </div>
        </div>
      )}

      {product.related?.length > 0 && <Related ids={product.related} />}
    </div>
  );
};

export default Product;
