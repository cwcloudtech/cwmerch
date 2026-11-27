import { useState } from "react";
import { Link } from "react-router-dom";
import { api, asArray, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import DateRange, { defaultRange } from "../../components/DateRange";
import { BarChart } from "../../components/Charts";
import { PageHead, SummaryCards } from "../../components/admin";
import { ErrorBox, Pagination, SelectField, Spinner, Tabs } from "../../components/ui";
import { productName, summaryCards } from "./Dashboard";

const Sales = ({ range }) => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [interval, setIntervalKind] = useState("day");
  const { data, loading, error } = useAsync(async () => {
    const [summary, sales] = await Promise.all([
      api.get("/v1/admin/reports/summary", { params: range }),
      api.get("/v1/admin/reports/sales", { params: { ...range, interval } }),
    ]);
    return { summary: summary.data, intervals: asArray(sales.data, "intervals") };
  }, [range.from, range.to, interval]);
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  const fmt = (d) => formatDate(d, interval === "month" ? { year: "numeric", month: "short" } : interval === "year" ? { year: "numeric" } : { month: "short", day: "numeric" });
  return (
    <div className="stack-lg">
      <SummaryCards cards={summaryCards(data.summary, money, t)} />
      <section className="card">
        <div className="section-head">
          <h2>{t("reports.salesOverTime")}</h2>
          <SelectField label={t("reports.interval")} value={interval} onChange={setIntervalKind} options={["day", "week", "month", "year"].map((x) => ({ value: x, label: t(`reports.intervals.${x}`) }))} />
        </div>
        <BarChart data={data.intervals} valueKey="grossSales" lineKey="netSales" format={money} formatLabel={fmt} title={t("reports.salesOverTime")} />
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{t("reports.date")}</th>
                <th scope="col" className="num">{t("reports.orders")}</th>
                <th scope="col" className="num">{t("reports.itemsSold")}</th>
                <th scope="col" className="num">{t("reports.grossSales")}</th>
                <th scope="col" className="num">{t("reports.refunds")}</th>
                <th scope="col" className="num">{t("reports.taxes")}</th>
                <th scope="col" className="num">{t("reports.shipping")}</th>
                <th scope="col" className="num">{t("reports.netSales")}</th>
              </tr>
            </thead>
            <tbody>
              {data.intervals.map((r) => (
                <tr key={r.date}>
                  <td>{fmt(r.date)}</td>
                  <td className="num">{r.orders}</td>
                  <td className="num">{r.itemsSold}</td>
                  <td className="num">{money(r.grossSales)}</td>
                  <td className="num">{money(r.refunds)}</td>
                  <td className="num">{money(r.taxes)}</td>
                  <td className="num">{money(r.shipping)}</td>
                  <td className="num">{money(r.netSales)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
};

const TopTable = ({ endpoint, range, kind }) => {
  const { t } = useI18n();
  const { money } = useShop();
  const { data, loading, error } = useAsync(async () => asArray((await api.get(endpoint, { params: { ...range, limit: 25 } })).data), [endpoint, range.from, range.to], []);
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  if (data.length === 0) return <p className="muted">{t("reports.noData")}</p>;
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            <th scope="col">#</th>
            <th scope="col">{kind === "product" ? t("cart.product") : t("product.categories")}</th>
            <th scope="col" className="num">{t("reports.itemsSold")}</th>
            <th scope="col" className="num">{t("reports.netSales")}</th>
            <th scope="col" className="num">{t("reports.orders")}</th>
          </tr>
        </thead>
        <tbody>
          {data.map((r, i) => (
            <tr key={r.productId || r.categoryId || r.id || i}>
              <td>{i + 1}</td>
              <td>{kind === "product" ? <Link to={`/admin/products/${r.productId || r.id}`}>{productName(r)}</Link> : r.name || r.categoryName}</td>
              <td className="num">{r.itemsSold ?? r.quantity ?? 0}</td>
              <td className="num">{money(r.netSales ?? r.total ?? 0)}</td>
              <td className="num">{r.orders ?? r.ordersCount ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

const Stock = () => {
  const { t } = useI18n();
  const [type, setType] = useState("low");
  const [page, setPage] = useState(1);
  const { data, loading, error } = useAsync(async () => normalizePaged((await api.get("/v1/admin/reports/stock", { params: { type, page } })).data, page), [type, page], { items: [], totalPages: 1 });
  return (
    <div className="stack">
      <Tabs
        tabs={["low", "out", "all"].map((x) => ({ id: x, label: t(`reports.stockType.${x}`) }))}
        active={type}
        onChange={(x) => {
          setType(x);
          setPage(1);
        }}
      />
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{t("cart.product")}</th>
                <th scope="col">{t("product.sku")}</th>
                <th scope="col">{t("admin.products.stockStatus")}</th>
                <th scope="col" className="num">{t("admin.products.stockQuantity")}</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((p) => (
                <tr key={p.id}>
                  <td>
                    <Link to={`/admin/products/${p.parentId || p.id}`}>{p.name}</Link>
                  </td>
                  <td>{p.sku}</td>
                  <td>
                    <span className={`stock ${p.stockStatus}`}>{t(`stock.${p.stockStatus}`)}</span>
                  </td>
                  <td className="num">{p.stockQuantity ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("reports.noData")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </div>
  );
};

const CustomersReport = ({ range }) => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [page, setPage] = useState(1);
  const { data, loading, error } = useAsync(async () => normalizePaged((await api.get("/v1/admin/reports/customers", { params: { ...range, page } })).data, page), [range.from, range.to, page], {
    items: [],
    totalPages: 1,
  });
  if (loading) return <Spinner />;
  if (error) return <ErrorBox error={error} />;
  return (
    <>
      <div className="table-wrap">
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{t("admin.customers.name")}</th>
              <th scope="col">{t("auth.email")}</th>
              <th scope="col" className="num">{t("admin.customers.orders")}</th>
              <th scope="col" className="num">{t("admin.customers.spent")}</th>
              <th scope="col">{t("admin.customers.lastOrder")}</th>
            </tr>
          </thead>
          <tbody>
            {data.items.map((c, i) => (
              <tr key={c.id || c.customerId || i}>
                <td>{c.id || c.customerId ? <Link to={`/admin/customers/${c.id || c.customerId}`}>{[c.firstName, c.lastName].filter(Boolean).join(" ") || c.name || c.email}</Link> : c.name || c.email}</td>
                <td>{c.email}</td>
                <td className="num">{c.ordersCount ?? c.orders ?? 0}</td>
                <td className="num">{money(c.totalSpent ?? c.netSales ?? 0)}</td>
                <td>{c.lastOrderAt ? formatDate(c.lastOrderAt) : "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {data.items.length === 0 && <p className="muted">{t("reports.noData")}</p>}
      </div>
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </>
  );
};

const Reports = () => {
  const { t } = useI18n();
  const [range, setRange] = useState(defaultRange);
  const [tab, setTab] = useState("sales");
  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.reports")} />
      <Tabs
        tabs={["sales", "products", "categories", "stock", "customers"].map((x) => ({ id: x, label: t(`reports.tabs.${x}`) }))}
        active={tab}
        onChange={setTab}
      />
      {tab !== "stock" && <DateRange value={range} onChange={setRange} />}
      {tab === "sales" && <Sales range={range} />}
      {tab === "products" && <TopTable endpoint="/v1/admin/reports/top-products" range={range} kind="product" />}
      {tab === "categories" && <TopTable endpoint="/v1/admin/reports/top-categories" range={range} kind="category" />}
      {tab === "stock" && <Stock />}
      {tab === "customers" && <CustomersReport range={range} />}
    </div>
  );
};

export default Reports;
