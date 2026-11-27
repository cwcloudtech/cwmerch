import { useLocation } from "react-router-dom";
import { Spinner } from "../../components/ui";
import { useShop } from "../../context/ShopContext";
import { routeKind, withDefaults } from "../../lib/permalinks";
import Catalogue from "./Catalogue";
import Product from "./Product";
import NotFound from "./NotFound";

// StorefrontRoute resolves the shop, product, category and tag pages from
// WooCommerce's permalink bases (they change with WordPress' settings and
// language: /produit/…, /categorie-produit/…, /boutique, %product_cat%…).
const StorefrontRoute = () => {
  const { pathname } = useLocation();
  const { settings, loaded } = useShop();
  if (!loaded) return <Spinner />;
  const { kind, ref } = routeKind(withDefaults(settings?.permalinks), pathname);
  switch (kind) {
    case "shop":
      return <Catalogue key="shop" mode="shop" />;
    case "category":
      // Hierarchical paths (parent/child): the last segment names the category.
      return <Catalogue key={`c-${ref}`} mode="category" slug={ref.split("/").pop()} />;
    case "tag":
      return <Catalogue key={`t-${ref}`} mode="tag" slug={ref.split("/").pop()} />;
    case "product":
      return <Product key={ref} slug={ref} />;
    default:
      return <NotFound />;
  }
};

export default StorefrontRoute;
