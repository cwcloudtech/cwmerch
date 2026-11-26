import { AddressView } from "./AddressForm";
import { StatusBadge } from "./ui";
import { useShop } from "../context/ShopContext";
import { useI18n } from "../i18n/I18nContext";

// OrderTotals renders WooCommerce's order totals block (subtotal, discount,
// shipping, fees, taxes, refunds, total) honouring taxDisplayCart.
export const OrderTotals = ({ order }) => {
  const { money, cartIncl, settings } = useShop();
  const { t } = useI18n();
  const subtotal = (order.lineItems || []).reduce((s, l) => s + Number(l.subtotal || 0) + (cartIncl ? Number(l.subtotalTax || 0) : 0), 0);
  const shipping = Number(order.shippingTotal || 0) + (cartIncl ? Number(order.shippingTax || 0) : 0);
  const discount = Number(order.discountTotal || 0) + (cartIncl ? Number(order.discountTax || 0) : 0);
  return (
    <table className="totals-table">
      <tbody>
        <tr>
          <th scope="row">{t("cart.subtotal")}</th>
          <td>{money(subtotal)}</td>
        </tr>
        {discount > 0 && (
          <tr>
            <th scope="row">{t("cart.discount")}</th>
            <td>-{money(discount)}</td>
          </tr>
        )}
        {(order.shippingLines || []).length > 0 && (
          <tr>
            <th scope="row">{t("cart.shipping")}</th>
            <td>
              {money(shipping)} <small className="muted">{(order.shippingLines || []).map((s) => s.methodTitle).join(", ")}</small>
            </td>
          </tr>
        )}
        {(order.feeLines || []).map((f, i) => (
          <tr key={`fee-${i}`}>
            <th scope="row">{f.name || t("order.fee")}</th>
            <td>{money(Number(f.total || 0) + (cartIncl ? Number(f.totalTax || 0) : 0))}</td>
          </tr>
        ))}
        {settings.calcTaxes &&
          !cartIncl &&
          (order.taxLines || []).map((tx) => (
            <tr key={tx.id || tx.rateId}>
              <th scope="row">{tx.label}</th>
              <td>{money(Number(tx.taxTotal || 0) + Number(tx.shippingTaxTotal || 0))}</td>
            </tr>
          ))}
        {order.paymentMethodTitle && (
          <tr>
            <th scope="row">{t("checkout.paymentMethod")}</th>
            <td>{order.paymentMethodTitle}</td>
          </tr>
        )}
        {(order.refunds || []).map((r) => (
          <tr key={r.id} className="refund-row">
            <th scope="row">{t("order.refund")}{r.reason ? ` (${r.reason})` : ""}</th>
            <td>-{money(Math.abs(Number(r.amount || 0)))}</td>
          </tr>
        ))}
        <tr className="grand-total">
          <th scope="row">{t("cart.total")}</th>
          <td>
            {money(order.total)}
            {settings.calcTaxes && cartIncl && Number(order.totalTax) > 0 && (
              <small className="muted"> {t("cart.includesTax", { tax: money(order.totalTax) })}</small>
            )}
          </td>
        </tr>
      </tbody>
    </table>
  );
};

const OrderDetails = ({ order }) => {
  const { money, cartIncl } = useShop();
  const { t, formatDate } = useI18n();
  if (!order) return null;
  return (
    <div className="order-details">
      <ul className="order-overview">
        <li>
          {t("order.number")} <strong>#{order.number || order.id}</strong>
        </li>
        <li>
          {t("order.date")} <strong>{formatDate(order.createdAt)}</strong>
        </li>
        <li>
          {t("order.status")} <StatusBadge status={order.status} label={t(`status.${order.status}`)} />
        </li>
        <li>
          {t("cart.total")} <strong>{money(order.total)}</strong>
        </li>
      </ul>
      <h2>{t("order.details")}</h2>
      <table className="table">
        <thead>
          <tr>
            <th scope="col">{t("cart.product")}</th>
            <th scope="col" className="num">
              {t("cart.total")}
            </th>
          </tr>
        </thead>
        <tbody>
          {(order.lineItems || []).map((l) => (
            <tr key={l.id}>
              <td>
                {l.name} <strong>× {l.quantity}</strong>
                {(l.meta || []).length > 0 && (
                  <ul className="item-meta">
                    {l.meta.map((m, i) => (
                      <li key={i}>
                        {m.key}: {m.value}
                      </li>
                    ))}
                  </ul>
                )}
              </td>
              <td className="num">{money(Number(l.subtotal || 0) + (cartIncl ? Number(l.subtotalTax || 0) : 0))}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <OrderTotals order={order} />
      {order.customerNote && (
        <p>
          <strong>{t("checkout.orderNotes")}:</strong> {order.customerNote}
        </p>
      )}
      <div className="two-col">
        <section>
          <h3>{t("address.billing")}</h3>
          <AddressView address={order.billing} />
        </section>
        <section>
          <h3>{t("address.shipping")}</h3>
          <AddressView address={order.shipping} />
        </section>
      </div>
    </div>
  );
};

export default OrderDetails;
