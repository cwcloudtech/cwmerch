import { Link } from "react-router-dom";
import { usePermalinks } from "../lib/permalinks";
import { toast } from "react-toastify";
import { FiShoppingBag } from "react-icons/fi";
import Price from "./Price";
import { Stars } from "./ui";
import { useCart } from "../context/CartContext";
import { useI18n } from "../i18n/I18nContext";

const ProductCard = ({ product }) => {
  const links = usePermalinks();
  const { addItem } = useCart();
  const { t } = useI18n();
  const image = product.images?.[0];
  const simple = product.type === "simple" && product.stockStatus !== "outofstock";

  const quickAdd = () => {
    addItem(product.id, 0, 1, product.soldIndividually);
    toast.success(t("cart.added", { name: product.name }));
  };

  return (
    <article className="product-card">
      <Link to={links.product(product)} className="product-card-media">
        {image ? <img src={image.src} alt={image.alt || product.name} loading="lazy" /> : <div className="img-placeholder" aria-hidden="true" />}
        {product.onSale && <span className="sale-flash">{t("product.sale")}</span>}
        {product.stockStatus === "outofstock" && <span className="oos-flash">{t("product.outOfStock")}</span>}
      </Link>
      <div className="product-card-body">
        <h3 className="product-card-title">
          <Link to={links.product(product)}>{product.name}</Link>
        </h3>
        {Number(product.ratingCount) > 0 && <Stars rating={product.averageRating} count={product.ratingCount} />}
        <Price item={product} />
      </div>
      <div className="product-card-actions">
        {simple ? (
          <button type="button" className="btn btn-primary btn-sm btn-block" onClick={quickAdd}>
            <FiShoppingBag aria-hidden="true" /> {t("product.addToCart")}
          </button>
        ) : product.type === "external" && product.externalUrl ? (
          <a className="btn btn-secondary btn-sm btn-block" href={product.externalUrl} target="_blank" rel="noopener noreferrer">
            {product.buttonText || t("product.buy")}
          </a>
        ) : (
          <Link className="btn btn-secondary btn-sm btn-block" to={links.product(product)}>
            {product.type === "variable" ? t("product.selectOptions") : t("product.viewProduct")}
          </Link>
        )}
      </div>
    </article>
  );
};

export const ProductGrid = ({ products }) => (
  <div className="product-grid">
    {products.map((p) => (
      <ProductCard key={p.id} product={p} />
    ))}
  </div>
);

export default ProductCard;
