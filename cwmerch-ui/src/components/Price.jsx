import { useShop } from "../context/ShopContext";
import { useI18n } from "../i18n/I18nContext";

// Price shows a product / variation price: sale strike-through, variable range,
// and the incl./excl. tax suffix that follows taxDisplayShop.
const Price = ({ item, range = true, className = "" }) => {
  const { money, settings, shopIncl } = useShop();
  const { t } = useI18n();
  if (!item) return null;
  const suffix = settings.calcTaxes ? <small className="price-suffix">{shopIncl ? t("price.inclTax") : t("price.exclTax")}</small> : null;
  const min = Number(item.priceMin ?? item.price);
  const max = Number(item.priceMax ?? item.price);
  if (range && (item.type === "variable" || item.type === "grouped") && Number.isFinite(min) && Number.isFinite(max) && min !== max) {
    return (
      <span className={`price ${className}`}>
        {money(min)} – {money(max)} {suffix}
      </span>
    );
  }
  const price = Number(item.price);
  const regular = Number(item.regularPrice);
  if (item.price === "" || item.price === null || item.price === undefined) return null;
  if (item.onSale && Number.isFinite(regular) && regular > price) {
    return (
      <span className={`price ${className}`}>
        <del aria-label={t("price.original")}>{money(regular)}</del> <ins aria-label={t("price.current")}>{money(price)}</ins> {suffix}
      </span>
    );
  }
  return (
    <span className={`price ${className}`}>
      {money(price)} {suffix}
    </span>
  );
};

export default Price;
