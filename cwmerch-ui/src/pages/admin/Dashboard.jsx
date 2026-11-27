import { useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import DateRange, { defaultRange } from "../../components/DateRange";
import { BarChart } from "../../components/Charts";
import { PageHead, SummaryCards } from "../../components/admin";
import { ErrorBox, Spinner, StatusBadge } from "../../components/ui";

export const summaryCards = (s, money, t) => [
  { label: t("reports.grossSales"), value: money(s?.grossSales) },
  { label: t("reports.netSales"), value: money(s?.netSales) },
  { label: t("reports.orders"), value: s?.orders ?? 0 },
  { label: t("reports.itemsSold"), value: s?.itemsSold ?? 0 },
  { label: t("reports.averageOrder"), value: money(s?.averageOrderValue) },
  { label: t("reports.refunds"), value: money(s?.refunds) },
  { label: t("reports.discount"), value: money(s?.discount) },
  { label: t("reports.newCustomers"), value: s?.newCustomers ?? 0 },
];

export const productName = (p) => p.name || p.productName || `#${p.productId || p.id}`;

const Dashboard = () => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [range, setRange] = useState(defaultRange);
  const { data, loading, error } = useAsync(async () => (await api.get("/v1/admin/dashboard", { params: range })).data, [range.from, range.to]);

  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.dashboard")} />
      <DateRange value={range} onChange={setRange} />
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        data && (
          <>
            <SummaryCards cards={summaryCards(data.summary, money, t)} />
            <section className="card">
              <h2>{t("reports.salesOverTime")}</h2>
              {asArray(data.sales, "intervals").length ? (
                <BarChart
                  data={asArray(data.sales, "intervals")}
                  valueKey="grossSales"
                  lineKey="netSales"
                  format={money}
                  formatLabel={(d) => formatDate(d, { month: "short", day: "numeric" })}
                  title={t("reports.salesOverTime")}
                />
              ) : (
                <p className="muted">{t("reports.noData")}</p>
              )}
              <p className="chart-legend small muted">
                <span className="legend-bar" /> {t("reports.grossSales")} <span className="legend-line" /> {t("reports.netSales")}
              </p>
            </section>
            <div className="two-col">
              <section className="card">
                <h2>{t("reports.topProducts")}</h2>
                <ol className="rank-list">
                  {asArray(data.topProducts).map((p, i) => (
                    <li key={p.productId || p.id || i}>
                      <Link to={`/admin/products/${p.productId || p.id}`}>{productName(p)}</Link>
                      <span className="muted small">
                        {p.itemsSold ?? p.quantity ?? 0} · {money(p.netSales ?? p.total ?? 0)}
                      </span>
                    </li>
                  ))}
                </ol>
                {asArray(data.topProducts).length === 0 && <p className="muted">{t("reports.noData")}</p>}
              </section>
              <section className="card">
                <h2>{t("reports.lowStock")}</h2>
                <ul className="plain-list">
                  {asArray(data.lowStock).map((p) => (
                    <li key={p.id} className="list-row">
                      <Link to={`/admin/products/${p.parentId || p.id}`}>{p.name}</Link>
                      <span className={`badge badge-${p.stockStatus === "outofstock" ? "failed" : "on-hold"}`}>{p.stockQuantity ?? t(`stock.${p.stockStatus}`)}</span>
                    </li>
                  ))}
                </ul>
                {asArray(data.lowStock).length === 0 && <p className="muted">{t("reports.noLowStock")}</p>}
              </section>
            </div>
            <section className="card">
              <div className="section-head">
                <h2>{t("reports.recentOrders")}</h2>
                <Link to="/admin/orders">{t("home.viewAll")}</Link>
              </div>
              <OrdersTable orders={asArray(data.recentOrders)} />
            </section>
          </>
        )
      )}
    </div>
  );
};

export const OrdersTable = ({ orders }) => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  if (!orders.length) return <p className="muted">{t("admin.orders.none")}</p>;
  return (
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
          </tr>
        </thead>
        <tbody>
          {orders.map((o) => (
            <tr key={o.id}>
              <td>
                <Link to={`/admin/orders/${o.id}`}>
                  #{o.number || o.id} {[o.billing?.firstName, o.billing?.lastName].filter(Boolean).join(" ")}
                </Link>
              </td>
              <td>{formatDate(o.createdAt)}</td>
              <td>
                <StatusBadge status={o.status} label={t(`status.${o.status}`)} />
              </td>
              <td className="num">{money(o.total)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

export default Dashboard;
