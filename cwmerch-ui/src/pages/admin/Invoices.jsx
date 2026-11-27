import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiDownload, FiFileText, FiMail, FiSettings } from "react-icons/fi";
import { api, downloadFile, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { formatMoney } from "../../lib/money";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { Empty, ErrorBox, Pagination, Spinner } from "../../components/ui";

// useInvoiceActions: download PDF / Markdown and email an invoice.
export const useInvoiceActions = () => {
  const { t, errMsg } = useI18n();
  const [busy, setBusy] = useState("");
  const run = async (key, fn) => {
    setBusy(key);
    try {
      await fn();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy("");
    }
  };
  return {
    busy,
    pdf: (inv) => run(`pdf-${inv.id}`, () => downloadFile(`/v1/admin/invoices/${inv.id}/pdf`, { fallbackName: `facture-${inv.number || inv.id}.pdf` })),
    markdown: (inv) => run(`md-${inv.id}`, () => downloadFile(`/v1/admin/invoices/${inv.id}/markdown`, { fallbackName: `facture-${inv.number || inv.id}.md` })),
    send: (inv) =>
      run(`send-${inv.id}`, async () => {
        await api.post(`/v1/admin/invoices/${inv.id}/send`, {});
        toast.success(t("invoice.sent"));
      }),
  };
};

export const InvoiceActions = ({ invoice, actions, labels = false }) => {
  const { t } = useI18n();
  const { busy, pdf, markdown, send } = actions;
  const cls = labels ? "btn btn-secondary btn-sm" : "icon-btn";
  return (
    <div className="row-actions">
      <button type="button" className={cls} onClick={() => pdf(invoice)} disabled={!!busy} aria-label={t("invoice.downloadPdf")} title={t("invoice.downloadPdf")}>
        <FiDownload aria-hidden="true" /> {labels && t("invoice.downloadPdf")}
      </button>
      <button type="button" className={cls} onClick={() => markdown(invoice)} disabled={!!busy} aria-label={t("invoice.downloadMarkdown")} title={t("invoice.downloadMarkdown")}>
        <FiFileText aria-hidden="true" /> {labels && t("invoice.downloadMarkdown")}
      </button>
      <button type="button" className={cls} onClick={() => send(invoice)} disabled={!!busy} aria-label={t("invoice.send")} title={t("invoice.send")}>
        <FiMail aria-hidden="true" /> {labels && t("invoice.send")}
      </button>
    </div>
  );
};

// Invoices: GET /v1/admin/invoices (paged, searchable).
const Invoices = () => {
  const { t, formatDate } = useI18n();
  const { settings } = useShop();
  const [params, setParams] = useSearchParams();
  const search = params.get("search") || "";
  const page = Number(params.get("page")) || 1;
  const actions = useInvoiceActions();
  const { data, loading, error } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/invoices", { params: { search: search || undefined, page, perPage: 20 } })).data, page),
    [search, page],
    { items: [], totalPages: 1, total: 0 }
  );
  const update = (changes) => {
    const next = new URLSearchParams(params);
    Object.entries(changes).forEach(([k, v]) => (v ? next.set(k, v) : next.delete(k)));
    if (!("page" in changes)) next.delete("page");
    setParams(next);
  };
  // Invoices carry their own currency: format with the shop's separators but that code.
  const money = (amount, currency) =>
    formatMoney(amount, currency && currency !== settings.currency ? { ...settings, currencySymbol: currency, currencyPosition: "right_space" } : settings);

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.invoices")} subtitle={data.total ? t("invoice.count", { count: data.total }) : undefined}>
        <Link to="/admin/settings/invoicing" className="btn btn-secondary btn-sm">
          <FiSettings aria-hidden="true" /> {t("invoice.settings")}
        </Link>
      </PageHead>
      <div className="toolbar">
        <span className="spacer" />
        <SearchBox value={search} onSearch={(s) => update({ search: s })} placeholder={t("invoice.search")} />
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : data.items.length === 0 ? (
        <Empty title={t("invoice.none")} body={t("invoice.noneHint")} />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{t("invoice.number")}</th>
                <th scope="col">{t("invoice.date")}</th>
                <th scope="col">{t("invoice.order")}</th>
                <th scope="col">{t("invoice.customer")}</th>
                <th scope="col" className="num">
                  {t("invoice.totalHT")}
                </th>
                <th scope="col" className="num">
                  {t("invoice.totalTax")}
                </th>
                <th scope="col" className="num">
                  {t("invoice.stamp")}
                </th>
                <th scope="col" className="num">
                  {t("invoice.totalTTC")}
                </th>
                <th scope="col">{t("invoice.currency")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((inv) => (
                <tr key={inv.id}>
                  <td className="nowrap">
                    <strong>{inv.number}</strong>
                  </td>
                  <td className="nowrap">{formatDate(inv.issuedAt)}</td>
                  <td>{inv.orderId ? <Link to={`/admin/orders/${inv.orderId}`}>#{inv.orderId}</Link> : "—"}</td>
                  <td>
                    {inv.customer?.name || "—"}
                    {inv.customer?.email && <div className="small muted">{inv.customer.email}</div>}
                  </td>
                  <td className="num">{money(inv.totalHT, inv.currency)}</td>
                  <td className="num">{money(inv.totalTax, inv.currency)}</td>
                  <td className="num">{Number(inv.stamp) ? money(inv.stamp, inv.currency) : "—"}</td>
                  <td className="num">
                    <strong>{money(inv.totalTTC, inv.currency)}</strong>
                  </td>
                  <td>{inv.currency}</td>
                  <td>
                    <InvoiceActions invoice={inv} actions={actions} />
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={(p) => update({ page: String(p) })} />
    </div>
  );
};

export default Invoices;
