import { useState } from "react";
import { toast } from "react-toastify";
import { FiDownload } from "react-icons/fi";
import { api, downloadFile } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { GenericTable, PageHead } from "../../components/admin";
import { ErrorBox, Spinner } from "../../components/ui";

const REPORTS = ["turnover", "balance", "ledger"];

// rowsOf digs the table rows out of an accounting JSON answer of unknown shape.
const rowsOf = (data) => {
  if (Array.isArray(data)) return data;
  if (!data || typeof data !== "object") return [];
  for (const key of ["months", "rows", "items", "lines", "accounts", "entries"]) if (Array.isArray(data[key])) return data[key];
  const firstArray = Object.values(data).find(Array.isArray);
  return firstArray || [];
};

const Accounting = () => {
  const { t, errMsg } = useI18n();
  const { money } = useShop();
  const now = new Date().getFullYear();
  const [year, setYear] = useState(now);
  const [busy, setBusy] = useState("");
  const { data, loading, error } = useAsync(async () => (await api.get("/v1/admin/accounting/turnover", { params: { year, format: "json" } })).data, [year]);

  const download = async (kind) => {
    setBusy(kind);
    try {
      await downloadFile(`/v1/admin/accounting/${kind}`, { params: { year, format: "pdf" }, fallbackName: `${kind}-${year}.pdf` });
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy("");
    }
  };

  const rows = rowsOf(data);
  const years = [];
  for (let y = now; y >= now - 10; y--) years.push(y);

  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.accounting")} subtitle={t("accounting.intro")} />
      <div className="row wrap">
        <label className="inline-field">
          <span>{t("accounting.year")}</span>
          <select className="input input-sm" value={year} onChange={(e) => setYear(Number(e.target.value))}>
            {years.map((y) => (
              <option key={y} value={y}>
                {y}
              </option>
            ))}
          </select>
        </label>
        {REPORTS.map((k) => (
          <button key={k} type="button" className="btn btn-secondary btn-sm" onClick={() => download(k)} disabled={!!busy}>
            <FiDownload aria-hidden="true" /> {t(`accounting.download.${k}`)}
          </button>
        ))}
      </div>
      <section className="card">
        <h2>{t("accounting.turnover", { year })}</h2>
        <ErrorBox error={error} />
        {loading ? (
          <Spinner />
        ) : rows.length === 0 ? (
          <p className="muted">{t("reports.noData")}</p>
        ) : (
          <GenericTable rows={rows} format={(v) => (typeof v === "number" && !Number.isInteger(v) ? money(v) : v ?? "—")} />
        )}
        {data && typeof data === "object" && !Array.isArray(data) && data.total !== undefined && (
          <p className="mt">
            <strong>{t("cart.total")}:</strong> {money(data.total)}
          </p>
        )}
      </section>
    </div>
  );
};

export default Accounting;
