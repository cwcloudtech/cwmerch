import { useSearchParams } from "react-router-dom";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { ORDER_STATUSES } from "../../lib/util";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { ErrorBox, Pagination, Spinner, Tabs } from "../../components/ui";
import { OrdersTable } from "./Dashboard";

const Orders = () => {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const status = params.get("status") || "";
  const search = params.get("search") || "";
  const page = Number(params.get("page")) || 1;
  const { data, loading, error } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/orders", { params: { status: status || undefined, search: search || undefined, page, perPage: 20 } })).data, page),
    [status, search, page],
    { items: [], totalPages: 1, total: 0 }
  );
  const update = (changes) => {
    const next = new URLSearchParams(params);
    Object.entries(changes).forEach(([k, v]) => (v ? next.set(k, v) : next.delete(k)));
    if (!("page" in changes)) next.delete("page");
    setParams(next);
  };
  return (
    <div className="stack">
      <PageHead title={t("admin.nav.orders")} />
      <div className="toolbar">
        <Tabs
          className="tabs-scroll"
          tabs={[{ id: "", label: t("common.all") }, ...ORDER_STATUSES.map((s) => ({ id: s, label: t(`status.${s}`) }))]}
          active={status}
          onChange={(s) => update({ status: s })}
        />
        <SearchBox value={search} onSearch={(s) => update({ search: s })} placeholder={t("admin.orders.search")} />
      </div>
      <ErrorBox error={error} />
      {loading ? <Spinner /> : <OrdersTable orders={data.items} />}
      <Pagination page={page} totalPages={data.totalPages} onChange={(p) => update({ page: String(p) })} />
    </div>
  );
};

export default Orders;
