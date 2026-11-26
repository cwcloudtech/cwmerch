import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import AddressForm, { EMPTY_ADDRESS } from "../../components/AddressForm";
import { PageHead, SearchBox } from "../../components/admin";
import { ErrorBox, Pagination, Spinner, TextField } from "../../components/ui";
import { OrdersTable } from "./Dashboard";
import { useAllCountries } from "./useAllCountries";

export const Customers = () => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const { data, loading, error } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/customers", { params: { page, perPage: 20, search: search || undefined } })).data, page),
    [page, search],
    { items: [], totalPages: 1 }
  );
  return (
    <div className="stack">
      <PageHead title={t("admin.nav.customers")} />
      <div className="toolbar">
        <span />
        <SearchBox
          value={search}
          onSearch={(s) => {
            setSearch(s);
            setPage(1);
          }}
          placeholder={t("admin.customers.search")}
        />
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table">
            <thead>
              <tr>
                <th scope="col">{t("admin.customers.name")}</th>
                <th scope="col">{t("admin.customers.username")}</th>
                <th scope="col">{t("auth.email")}</th>
                <th scope="col">{t("admin.customers.location")}</th>
                <th scope="col" className="num">
                  {t("admin.customers.orders")}
                </th>
                <th scope="col" className="num">
                  {t("admin.customers.spent")}
                </th>
                <th scope="col">{t("admin.customers.lastOrder")}</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((c) => (
                <tr key={c.id}>
                  <td>
                    <Link to={`/admin/customers/${c.id}`}>{[c.firstName, c.lastName].filter(Boolean).join(" ") || c.username || `#${c.id}`}</Link>
                  </td>
                  <td>{c.username}</td>
                  <td>
                    <a href={`mailto:${c.email}`}>{c.email}</a>
                  </td>
                  <td className="small">{[c.billing?.city, c.billing?.country].filter(Boolean).join(", ")}</td>
                  <td className="num">{c.ordersCount ?? 0}</td>
                  <td className="num">{money(c.totalSpent)}</td>
                  <td>{c.lastOrderAt ? formatDate(c.lastOrderAt) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("admin.customers.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </div>
  );
};

export const CustomerDetail = () => {
  const { id } = useParams();
  const { t, errMsg, formatDate } = useI18n();
  const { money } = useShop();
  const countries = useAllCountries();
  const [c, setC] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const { data: orders } = useAsync(async () => normalizePaged((await api.get(`/v1/admin/customers/${id}/orders`)).data).items, [id], []);

  useEffect(() => {
    api
      .get(`/v1/admin/customers/${id}`)
      .then(({ data }) => setC({ ...data, billing: { ...EMPTY_ADDRESS, ...data.billing }, shipping: { ...EMPTY_ADDRESS, ...data.shipping } }))
      .catch(setError);
  }, [id]);

  if (!c) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setC((x) => ({ ...x, [k]: v }));

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.put(`/v1/admin/customers/${id}`, { firstName: c.firstName, lastName: c.lastName, email: c.email, billing: c.billing, shipping: c.shipping });
      toast.success(t("admin.saved"));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack-lg">
      <PageHead title={[c.firstName, c.lastName].filter(Boolean).join(" ") || c.username || c.email} subtitle={`${c.email} · ${t("admin.customers.since", { date: formatDate(c.createdAt) })}`}>
        <Link to="/admin/customers" className="btn btn-secondary btn-sm">
          {t("common.back")}
        </Link>
      </PageHead>
      <div className="summary-cards">
        <div className="summary-card">
          <span className="summary-label">{t("admin.customers.orders")}</span>
          <strong className="summary-value">{c.ordersCount ?? orders.length}</strong>
        </div>
        <div className="summary-card">
          <span className="summary-label">{t("admin.customers.spent")}</span>
          <strong className="summary-value">{money(c.totalSpent)}</strong>
        </div>
        <div className="summary-card">
          <span className="summary-label">{t("admin.customers.role")}</span>
          <strong className="summary-value">{t(`roles.${c.role}`)}</strong>
        </div>
      </div>
      <form className="stack-lg" onSubmit={save}>
        <section className="card form-grid">
          <TextField label={t("auth.firstName")} value={c.firstName} onChange={set("firstName")} />
          <TextField label={t("auth.lastName")} value={c.lastName} onChange={set("lastName")} />
          <TextField className="span-2" label={t("auth.email")} type="email" value={c.email} onChange={set("email")} required />
        </section>
        <div className="two-col">
          <section className="card">
            <h2>{t("address.billing")}</h2>
            <AddressForm value={c.billing} onChange={set("billing")} withEmail countries={countries} required={false} idPrefix="billing" />
          </section>
          <section className="card">
            <h2>{t("address.shipping")}</h2>
            <AddressForm value={c.shipping} onChange={set("shipping")} countries={countries} required={false} idPrefix="shipping" />
          </section>
        </div>
        <div>
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("common.saveChanges")}
          </button>
        </div>
      </form>
      <section className="card">
        <h2>{t("admin.nav.orders")}</h2>
        <OrdersTable orders={orders} />
      </section>
    </div>
  );
};
