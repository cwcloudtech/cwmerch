import { useLocation, useParams, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useSeo } from "../../lib/seo";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import OrderDetails from "../../components/OrderDetails";
import { ErrorBox, Spinner } from "../../components/ui";

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

const OrderReceived = () => {
  const { id } = useParams();
  const [params] = useSearchParams();
  const location = useLocation();
  const { t } = useI18n();
  const { shopName } = useShop();
  const key = params.get("key") || "";
  useSeo({ title: `${t("checkout.orderReceived")} – ${shopName}`, noindex: true });
  const { data, loading, error } = useAsync(async () => (await api.get(`/v1/shop/orders/${id}`, { params: { key } })).data, [id, key]);
  const order = data?.order || (data?.id ? data : null) || location.state?.order;
  const instructions = data?.paymentInstructions ?? location.state?.paymentInstructions;
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
