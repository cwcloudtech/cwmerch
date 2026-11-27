// WooCommerce permalinks (WordPress → Settings → Permalinks), served by the
// API in the shop settings: product, category, tag and shop pages have the
// same addresses on cwmerch as on WordPress. Mirrors woo.Permalinks (Go).
import { useMemo } from "react";
import { useShop } from "../context/ShopContext";

export const DEFAULT_PERMALINKS = {
  productBase: "product",
  categoryBase: "product-category",
  tagBase: "product-tag",
  shopBase: "shop",
  productCategoryInPath: false,
};

// App routes that are never storefront pages.
const RESERVED = new Set(["admin", "cart", "checkout", "contact", "my-account", "forgot-password", "reset-password", "oidc", "assistant", "v1"]);

const trim = (s) => String(s || "").replace(/^\/+|\/+$/g, "");

export const withDefaults = (pl) => ({ ...DEFAULT_PERMALINKS, ...(pl || {}) });

export const shopPath = (pl) => `/${trim(withDefaults(pl).shopBase)}`;

export const categoryPath = (pl, term) => {
  if (!term) return shopPath(pl);
  if (term.path) return term.path;
  return `/${withDefaults(pl).categoryBase}/${term.slug || term.id}`;
};

export const tagPath = (pl, term) => {
  if (!term) return shopPath(pl);
  if (term.path) return term.path;
  return `/${withDefaults(pl).tagBase}/${term.slug || term.id}`;
};

// productPath prefers the API's own path (it resolves %product_cat%); a bare
// slug or id still resolves because the last segment names the product.
export const productPath = (pl, product) => {
  if (product?.path) return product.path;
  const p = withDefaults(pl);
  const ref = typeof product === "object" ? product?.slug || product?.id : product;
  if (p.productCategoryInPath) {
    const prefix = trim(p.productBase.split("%product_cat%")[0]);
    return `/${prefix ? `${prefix}/` : ""}uncategorized/${ref}`;
  }
  return `/${trim(p.productBase)}/${ref}`;
};

// routeKind tells what a storefront path is (same rules as the API's
// Permalinks.RouteKind): home, shop, product, category, tag or "".
export const routeKind = (pl, path) => {
  const p = withDefaults(pl);
  const clean = trim(decodeURI(path || ""));
  if (!clean) return { kind: "home" };
  if (RESERVED.has(clean.split("/")[0])) return { kind: "" };
  if (clean === trim(p.shopBase)) return { kind: "shop" };
  const cat = trim(p.categoryBase) + "/";
  if (clean.startsWith(cat) && clean.length > cat.length) return { kind: "category", ref: clean.slice(cat.length) };
  const tag = trim(p.tagBase) + "/";
  if (clean.startsWith(tag) && clean.length > tag.length) return { kind: "tag", ref: clean.slice(tag.length) };
  const prefix = p.productCategoryInPath ? trim(p.productBase.split("%product_cat%")[0]) : trim(p.productBase);
  const segs = clean.split("/");
  if (prefix === "" || clean.startsWith(prefix + "/")) {
    if (segs.length >= 2 || prefix === "") return { kind: "product", ref: segs[segs.length - 1] };
  }
  return { kind: "" };
};

export const usePermalinks = () => {
  const { settings } = useShop();
  const raw = settings?.permalinks;
  return useMemo(() => {
    const pl = withDefaults(raw);
    return {
      pl,
      shop: shopPath(pl),
      product: (p) => productPath(pl, p),
      category: (t) => categoryPath(pl, t),
      tag: (t) => tagPath(pl, t),
    };
  }, [raw]);
};
