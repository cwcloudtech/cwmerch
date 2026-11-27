import { useEffect, useState } from "react";
import { usePermalinks } from "../../lib/permalinks";
import { FiDownload } from "react-icons/fi";
import { Link, useParams } from "react-router-dom";
import { api, fetchBlob, normalizePaged, saveBlob } from "../../api/client";
import { PayNowButton } from "../shop/Payment";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import OrderDetails from "../../components/OrderDetails";
import { Empty, ErrorBox, Pagination, Spinner, StatusBadge } from "../../components/ui";

export const Orders = () => {
  const links = usePermalinks();
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [page, setPage] = useState(1);
  const { data, loading, error } = useAsync(async () => normalizePaged((await api.get("/v1/users/me/orders", { params: { page, perPage: 10 } })).data, page, 10), [page], {
    items: [],
    totalPages: 1,
  });
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  if (data.items.length === 0)
    return (
      <Empty title={t("account.noOrders")}>
        <Link to={links.shop} className="btn btn-primary">
          {t("cart.returnToShop")}
        </Link>
      </Empty>
    );
  return (
    <>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{t("order.number")}</th>
              <th scope="col">{t("order.date")}</th>
              <th scope="col">{t("order.status")}</th>
              <th scope="col" className="num">
                {t("cart.total")}
              </th>
              <th scope="col">
                <span className="sr-only">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((o) => (
              <tr key={o.id}>
                <td>
                  <Link to={`/my-account/view-order/${o.id}`}>#{o.number || o.id}</Link>
                </td>
                <td>{formatDate(o.createdAt)}</td>
                <td>
                  <StatusBadge status={o.status} label={t(`status.${o.status}`)} />
                </td>
                <td className="num">
                  {money(o.total)} <span className="muted small">{t("account.forItems", { count: (o.lineItems || []).reduce((n, l) => n + Number(l.quantity || 0), 0) })}</span>
                </td>
                <td>
                  <Link className="btn btn-secondary btn-sm" to={`/my-account/view-order/${o.id}`}>
                    {t("common.view")}
                  </Link>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </>
  );
};

// InvoiceButton probes GET /v1/users/me/orders/{id}/invoice once (the PDF is
// kept for the click) and stays hidden when the order has no invoice (404).
const InvoiceButton = ({ orderId }) => {
  const { t } = useI18n();
  const [file, setFile] = useState(null);
  useEffect(() => {
    let cancelled = false;
    setFile(null);
    fetchBlob(`/v1/users/me/orders/${orderId}/invoice`, { fallbackName: `invoice-${orderId}.pdf` })
      .then((f) => !cancelled && setFile(f))
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [orderId]);
  if (!file) return null;
  return (
    <button type="button" className="btn btn-secondary btn-sm" onClick={() => saveBlob(file.blob, file.name)}>
      <FiDownload aria-hidden="true" /> {t("invoice.download")}
    </button>
  );
};

export const ViewOrder = () => {
  const { id } = useParams();
  const { data, loading, error } = useAsync(async () => (await api.get(`/v1/users/me/orders/${id}`)).data, [id]);
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  const order = data?.order || data;
  return (
    <div className="stack">
      <PayNowButton order={order} />
      <div className="row end">
        <InvoiceButton orderId={id} />
      </div>
      <OrderDetails order={order} />
    </div>
  );
};

export const Downloads = () => {
  const links = usePermalinks();
  const { t, formatDate } = useI18n();
  const { data, loading, error } = useAsync(async () => (await api.get("/v1/users/me/downloads")).data || [], [], []);
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  const list = Array.isArray(data) ? data : data.items || [];
  if (list.length === 0) return <Empty title={t("account.noDownloads")} />;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th scope="col">{t("cart.product")}</th>
            <th scope="col">{t("account.downloadsRemaining")}</th>
            <th scope="col">{t("account.expires")}</th>
            <th scope="col">{t("account.download")}</th>
          </tr>
        </thead>
        <tbody>
          {list.map((d) => (
            <tr key={`${d.productId}-${d.downloadId}`}>
              <td>
                <Link to={links.product(d.productId)}>{d.productName}</Link>
              </td>
              <td>{d.remaining === null || d.remaining === undefined || Number(d.remaining) < 0 ? "∞" : d.remaining}</td>
              <td>{d.expiresAt ? formatDate(d.expiresAt) : t("account.never")}</td>
              <td>
                <a className="btn btn-secondary btn-sm" href={d.url} rel="noopener noreferrer">
                  {d.name}
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};
