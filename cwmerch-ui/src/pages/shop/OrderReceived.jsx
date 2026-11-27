import { toast } from "react-toastify";
import { FiCopy, FiPhone } from "react-icons/fi";
import { useLocation, useParams, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useSeo } from "../../lib/seo";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import OrderDetails from "../../components/OrderDetails";
import { ErrorBox, Spinner } from "../../components/ui";
import { PayNowButton } from "./Payment";

// PaymentInstructions renders the gateway's instructions: plain text, or an
// object carrying {instructions, accounts:[...]} for direct bank transfer.
export const PaymentInstructions = ({ value }) => {
  const { t } = useI18n();
  if (!value) return null;
  if (typeof value === "string") {
    return (
      <section className="card payment-instructions">
        <h2>{t("checkout.paymentInstructions")}</h2>
        <p className="pre-line">{value}</p>
      </section>
    );
  }
  const accounts = value.accounts || [];
  return (
    <section className="card payment-instructions">
      <h2>{value.title || t("checkout.paymentInstructions")}</h2>
      {value.instructions && <p className="pre-line">{value.instructions}</p>}
      {accounts.map((a, i) => (
        <dl key={i} className="bank-account">
          {a.accountName && (
            <>
              <dt>{t("payments.accountName")}</dt>
              <dd>{a.accountName}</dd>
            </>
          )}
          {a.bankName && (
            <>
              <dt>{t("payments.bankName")}</dt>
              <dd>{a.bankName}</dd>
            </>
          )}
          {a.accountNumber && (
            <>
              <dt>{t("payments.accountNumber")}</dt>
              <dd>{a.accountNumber}</dd>
            </>
          )}
          {a.sortCode && (
            <>
              <dt>{t("payments.sortCode")}</dt>
              <dd>{a.sortCode}</dd>
            </>
          )}
          {a.iban && (
            <>
              <dt>IBAN</dt>
              <dd>{a.iban}</dd>
            </>
          )}
          {a.bic && (
            <>
              <dt>BIC</dt>
              <dd>{a.bic}</dd>
            </>
          )}
        </dl>
      ))}
    </section>
  );
};

// PhoneConfirmation: no payment partner — the order number to keep and the
// phone number the shop will call to confirm the order.
export const PhoneConfirmation = ({ orderNumber, phone, message }) => {
  const { t } = useI18n();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(String(orderNumber));
      toast.success(t("phoneConfirm.copied"));
    } catch {
      /* clipboard unavailable: the number stays selectable */
    }
  };
  return (
    <section className="card phone-confirmation stack" aria-labelledby="phone-confirmation-title">
      <h2 id="phone-confirmation-title">{t("phoneConfirm.orderNumber")}</h2>
      <div className="order-number-big">
        <output aria-live="polite">{orderNumber}</output>
        <button type="button" className="btn btn-secondary btn-sm" onClick={copy} aria-label={t("phoneConfirm.copy")} title={t("phoneConfirm.copy")}>
          <FiCopy aria-hidden="true" /> {t("phoneConfirm.copy")}
        </button>
      </div>
      <p className="phone-confirmation-msg">
        <FiPhone aria-hidden="true" /> {message || (phone ? t("phoneConfirm.willCall", { phone }) : t("phoneConfirm.willCallNoPhone"))}
      </p>
      <p className="small muted">{t("phoneConfirm.keepNumber")}</p>
    </section>
  );
};

const OrderReceived = () => {
  const { id } = useParams();
  const [params] = useSearchParams();
  const location = useLocation();
  const { t } = useI18n();
  const { shopName, settings } = useShop();
  const key = params.get("key") || "";
  useSeo({ title: `${t("checkout.orderReceived")} – ${shopName}`, noindex: true });
  const { data, loading, error } = useAsync(async () => (await api.get(`/v1/shop/orders/${id}`, { params: { key } })).data, [id, key]);
  const order = data?.order || (data?.id ? data : null) || location.state?.order;
  const instructions = data?.paymentInstructions ?? location.state?.paymentInstructions;
  const confirmation =
    location.state?.confirmation ||
    data?.confirmation ||
    (settings.phoneConfirmation && order?.status === "on-hold" && !order?.paymentMethod
      ? { orderNumber: order.number || order.id, phone: order.billing?.phone || "" }
      : null);
  return (
    <div className="container order-received">
      <h1>{t("checkout.orderReceived")}</h1>
      {loading && !order ? (
        <Spinner />
      ) : (
        <>
          {error && !order && <ErrorBox error={error} />}
          {order && (
            <>
              <p className="notice notice-success">{t("checkout.thankYou")}</p>
              {confirmation && <PhoneConfirmation orderNumber={confirmation.orderNumber || order.number || order.id} phone={confirmation.phone} message={confirmation.message} />}
              <PayNowButton order={order} />
              <PaymentInstructions value={instructions} />
              <OrderDetails order={order} />
            </>
          )}
        </>
      )}
    </div>
  );
};

export default OrderReceived;
