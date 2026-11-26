import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import OrderDetails from "../../components/OrderDetails";
import { Empty, ErrorBox, Pagination, Spinner, StatusBadge } from "../../components/ui";

export const Orders = () => {
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
        <Link to="/shop" className="btn btn-primary">
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

export const ViewOrder = () => {
  const { id } = useParams();
  const { data, loading, error } = useAsync(async () => (await api.get(`/v1/users/me/orders/${id}`)).data, [id]);
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  return <OrderDetails order={data?.order || data} />;
};

export const Downloads = () => {
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
                <Link to={`/product/${d.productId}`}>{d.productName}</Link>
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
