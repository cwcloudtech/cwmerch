import { useState } from "react";
import { usePermalinks } from "../../lib/permalinks";
import { Link } from "react-router-dom";
import { FiTrash2 } from "react-icons/fi";
import { useCart } from "../../context/CartContext";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import Img from "../../components/Img";
import { Empty, ErrorBox, Field, Spinner } from "../../components/ui";

export const CartNotices = ({ notices }) =>
  (notices || []).length > 0 && (
    <div className="notices" aria-live="polite">
      {notices.map((n, i) => (
        <div key={i} className={`notice ${n.type === "error" ? "notice-error" : "notice-info"}`} role={n.type === "error" ? "alert" : "status"}>
          {n.message}
        </div>
      ))}
    </div>
  );

// ShippingRates is the radio list of shipping methods returned by the cart pricing.
export const ShippingRates = ({ cart, onChange, name = "shipping-method" }) => {
  const { money, cartIncl } = useShop();
  const { t } = useI18n();
  const rates = cart?.shippingRates || [];
  if (!cart?.needsShipping) return null;
  if (rates.length === 0) return <p className="muted small">{t("cart.noShippingMethods")}</p>;
  return (
    <fieldset className="radio-list">
      <legend className="sr-only">{t("cart.shippingMethod")}</legend>
      {rates.map((r) => (
        <label key={r.id} className="radio-row">
          <input type="radio" name={name} value={r.id} checked={cart.chosenShippingMethod === r.id} onChange={() => onChange(r.id)} />
          <span>
            <bdi>{r.label}</bdi>
            {Number(r.cost) > 0 && `: ${money(Number(r.cost) + (cartIncl ? Number(r.tax || 0) : 0))}`}
          </span>
        </label>
      ))}
    </fieldset>
  );
};

// CartTotals renders subtotal / discounts / shipping / taxes / total for a priced cart.
export const CartTotals = ({ cart, showShipping = true, onShippingChange, onRemoveCoupon }) => {
  const { money, cartIncl, settings } = useShop();
  const { t } = useI18n();
  if (!cart) return null;
  const subtotal = Number(cart.subtotal || 0) + (cartIncl ? Number(cart.subtotalTax || 0) : 0);
  const shipping = Number(cart.shippingTotal || 0) + (cartIncl ? Number(cart.shippingTax || 0) : 0);
  return (
    <table className="totals-table">
      <tbody>
        <tr>
          <th scope="row">{t("cart.subtotal")}</th>
          <td>{money(subtotal)}</td>
        </tr>
        {(cart.coupons || []).map((c) => (
          <tr key={c.code}>
            <th scope="row">{t("cart.coupon", { code: c.code })}</th>
            <td>
              -{money(Number(c.discount || 0) + (cartIncl ? Number(c.discountTax || 0) : 0))}{" "}
              {onRemoveCoupon && (
                <button type="button" className="link-btn small" onClick={() => onRemoveCoupon(c.code)}>
                  {t("cart.removeCoupon")}
                </button>
              )}
            </td>
          </tr>
        ))}
        {showShipping && cart.needsShipping && (
          <tr>
            <th scope="row">{t("cart.shipping")}</th>
            <td>{onShippingChange ? <ShippingRates cart={cart} onChange={onShippingChange} /> : money(shipping)}</td>
          </tr>
        )}
        {(cart.fees || []).map((f, i) => (
          <tr key={`fee-${i}`} className="fee-row">
            <th scope="row">{f.name || t("order.fee")}</th>
            <td>{money(Number(f.total || 0) + (cartIncl ? Number(f.totalTax || 0) : 0))}</td>
          </tr>
        ))}
        {settings.calcTaxes &&
          !cartIncl &&
          (cart.taxes || []).map((tx) => (
            <tr key={tx.rateId}>
              <th scope="row">{tx.label}</th>
              <td>{money(tx.amount)}</td>
            </tr>
          ))}
        <tr className="grand-total">
          <th scope="row">{t("cart.total")}</th>
          <td>
            <strong>{money(cart.total)}</strong>
            {settings.calcTaxes && cartIncl && Number(cart.taxTotal) > 0 && <small className="muted"> {t("cart.includesTax", { tax: money(cart.taxTotal) })}</small>}
          </td>
        </tr>
      </tbody>
    </table>
  );
};

const ShippingCalculator = () => {
  const { estimate, setCustomer } = useCart();
  const { countries } = useShop();
  const { t } = useI18n();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState(estimate.customer);
  const states = countries.find((c) => c.code === draft.country)?.states || [];
  return (
    <div className="shipping-calc">
      <button type="button" className="link-btn" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        {t("cart.calculateShipping")}
      </button>
      {open && (
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault();
            setCustomer(draft);
          }}
        >
          <Field label={t("address.country")}>
            {(id) => (
              <select id={id} className="input" value={draft.country} onChange={(e) => setDraft({ ...draft, country: e.target.value, state: "" })}>
                <option value="">{t("address.selectCountry")}</option>
                {countries.map((c) => (
                  <option key={c.code} value={c.code}>
                    {c.name}
                  </option>
                ))}
              </select>
            )}
          </Field>
          {states.length > 0 && (
            <Field label={t("address.state")}>
              {(id) => (
                <select id={id} className="input" value={draft.state} onChange={(e) => setDraft({ ...draft, state: e.target.value })}>
                  <option value="">{t("address.selectState")}</option>
                  {states.map((s) => (
                    <option key={s.code} value={s.code}>
                      {s.name}
                    </option>
                  ))}
                </select>
              )}
            </Field>
          )}
          <Field label={t("address.city")}>{(id) => <input id={id} className="input" value={draft.city} onChange={(e) => setDraft({ ...draft, city: e.target.value })} />}</Field>
          <Field label={t("address.postcode")}>{(id) => <input id={id} className="input" value={draft.postcode} onChange={(e) => setDraft({ ...draft, postcode: e.target.value })} />}</Field>
          <button type="submit" className="btn btn-secondary btn-sm">
            {t("cart.update")}
          </button>
        </form>
      )}
    </div>
  );
};

const Cart = () => {
  const links = usePermalinks();
  const { items, cart, pricing, error, setQuantity, removeItem, applyCoupon, removeCoupon, setShippingMethod } = useCart();
  const { settings, money, cartIncl, shopName } = useShop();
  const { t } = useI18n();
  const [coupon, setCoupon] = useState("");
  useSeo({ title: `${t("cart.title")} – ${shopName}`, noindex: true });

  if (items.length === 0) {
    return (
      <div className="container">
        <h1>{t("cart.title")}</h1>
        <Empty title={t("cart.empty")} body={t("cart.emptyBody")}>
          <Link to={links.shop} className="btn btn-primary">
            {t("cart.returnToShop")}
          </Link>
        </Empty>
      </div>
    );
  }

  return (
    <div className="container cart-page">
      <h1>{t("cart.title")}</h1>
      <CartNotices notices={cart?.notices} />
      <ErrorBox error={error} />
      <div className="cart-layout">
        <div>
          {!cart && pricing ? (
            <Spinner />
          ) : (
            <table className="table cart-table">
              <thead>
                <tr>
                  <th scope="col">{t("cart.product")}</th>
                  <th scope="col" className="num hide-mobile">
                    {t("cart.price")}
                  </th>
                  <th scope="col">{t("cart.quantity")}</th>
                  <th scope="col" className="num">
                    {t("cart.subtotal")}
                  </th>
                  <th scope="col">
                    <span className="sr-only">{t("common.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {(cart?.items || []).map((line) => {
                  const lineTotal = Number(line.subtotal || 0) + (cartIncl ? Number(line.subtotalTax || 0) : 0);
                  const unit = line.quantity ? lineTotal / line.quantity : line.price;
                  return (
                    <tr key={line.key}>
                      <td>
                        <div className="cart-product">
                          <Img src={line.image} alt="" width="56" height="56" />
                          <div>
                            <Link to={links.product(line.productId)}>{line.name}</Link>
                            {(line.attributes || []).length > 0 && (
                              <ul className="item-meta">
                                {line.attributes.map((a) => (
                                  <li key={a.name}>
                                    {a.name}: {a.option}
                                  </li>
                                ))}
                              </ul>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="num hide-mobile">{money(unit)}</td>
                      <td>
                        <label className="sr-only" htmlFor={`qty-${line.key}`}>
                          {t("product.quantityFor", { name: line.name })}
                        </label>
                        <input
                          id={`qty-${line.key}`}
                          className="input qty-input"
                          type="number"
                          min="0"
                          max={line.maxQuantity > 0 ? line.maxQuantity : undefined}
                          value={line.quantity}
                          onChange={(e) => setQuantity(line.productId, line.variationId, e.target.value)}
                        />
                      </td>
                      <td className="num">{money(lineTotal)}</td>
                      <td>
                        <button type="button" className="icon-btn danger" onClick={() => removeItem(line.productId, line.variationId)} aria-label={t("cart.removeItem", { name: line.name })}>
                          <FiTrash2 />
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          {settings.enableCoupons && (
            <form
              className="coupon-form"
              onSubmit={(e) => {
                e.preventDefault();
                applyCoupon(coupon);
                setCoupon("");
              }}
            >
              <label htmlFor="coupon" className="sr-only">
                {t("cart.couponCode")}
              </label>
              <input id="coupon" className="input" placeholder={t("cart.couponCode")} value={coupon} onChange={(e) => setCoupon(e.target.value)} />
              <button type="submit" className="btn btn-secondary" disabled={!coupon.trim()}>
                {t("cart.applyCoupon")}
              </button>
            </form>
          )}
        </div>
        <aside className="card cart-totals" aria-busy={pricing}>
          <h2>{t("cart.totals")}</h2>
          <CartTotals cart={cart} onShippingChange={setShippingMethod} onRemoveCoupon={removeCoupon} />
          {cart?.needsShipping && <ShippingCalculator />}
          {settings.phoneConfirmation && (
            <p className="notice notice-info phone-confirm-notice" role="note">
              {t("phoneConfirm.cartNotice")}
            </p>
          )}
          <Link to="/checkout" className="btn btn-primary btn-lg btn-block">
            {t("cart.proceed")}
          </Link>
          {pricing && <p className="small muted" aria-live="polite">{t("cart.updating")}</p>}
        </aside>
      </div>
    </div>
  );
};

export default Cart;
