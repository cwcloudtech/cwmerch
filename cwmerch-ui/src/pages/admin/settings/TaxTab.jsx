import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { FiPlus, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { listToText, textToList } from "../../../lib/util";
import { useI18n } from "../../../i18n/I18nContext";
import { ConfirmModal, ErrorBox, Spinner, Tabs } from "../../../components/ui";

const blankRate = (cls) => ({ id: 0, country: "", state: "", postcodes: [], cities: [], rate: "", name: "", priority: 1, compound: false, shipping: true, order: 0, class: cls, _new: true });

// RatesTable is WooCommerce's editable tax-rate grid for one class.
const RatesTable = ({ cls }) => {
  const { t, errMsg } = useI18n();
  const { data, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/taxes", { params: { class: cls } })).data), [cls], []);
  const [rows, setRows] = useState([]);
  const [removed, setRemoved] = useState([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setRows(data.map((r) => ({ ...r, _postcodes: listToText(r.postcodes, "; "), _cities: listToText(r.cities, "; ") })));
    setRemoved([]);
  }, [data]);

  const set = (i, patch) => setRows((rs) => rs.map((r, j) => (j === i ? { ...r, ...patch, _dirty: true } : r)));

  const saveAll = async () => {
    setBusy(true);
    try {
      for (const id of removed) await api.delete(`/v1/admin/taxes/${id}`);
      for (const [i, r] of rows.entries()) {
        if (!r._dirty && !r._new) continue;
        const { _dirty, _new, _postcodes, _cities, ...rest } = r;
        const body = {
          ...rest,
          country: String(rest.country || "").toUpperCase(),
          state: String(rest.state || "").toUpperCase(),
          postcodes: textToList(_postcodes, /[;\n]+/),
          cities: textToList(_cities, /[;\n]+/),
          rate: Number(rest.rate || 0),
          priority: Number(rest.priority || 1),
          order: i,
          class: cls,
        };
        if (_new) await api.post("/v1/admin/taxes", body);
        else await api.put(`/v1/admin/taxes/${r.id}`, body);
      }
      toast.success(t("admin.saved"));
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Spinner />;
  return (
    <div className="stack">
      <ErrorBox error={error} />
      <div className="table-wrap">
        <table className="table tax-table">
          <thead>
            <tr>
              <th scope="col">{t("tax.country")}</th>
              <th scope="col">{t("tax.state")}</th>
              <th scope="col">{t("tax.postcodes")}</th>
              <th scope="col">{t("tax.cities")}</th>
              <th scope="col">{t("tax.rate")}</th>
              <th scope="col">{t("tax.name")}</th>
              <th scope="col">{t("tax.priority")}</th>
              <th scope="col">{t("tax.compound")}</th>
              <th scope="col">{t("tax.shipping")}</th>
              <th scope="col">
                <span className="sr-only">{t("common.actions")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={r.id || `new-${i}`}>
                <td>
                  <input className="input input-sm w-xs" aria-label={t("tax.country")} value={r.country} onChange={(e) => set(i, { country: e.target.value })} placeholder="*" maxLength={2} />
                </td>
                <td>
                  <input className="input input-sm w-xs" aria-label={t("tax.state")} value={r.state} onChange={(e) => set(i, { state: e.target.value })} placeholder="*" />
                </td>
                <td>
                  <input className="input input-sm" aria-label={t("tax.postcodes")} value={r._postcodes ?? ""} onChange={(e) => set(i, { _postcodes: e.target.value })} placeholder="*" />
                </td>
                <td>
                  <input className="input input-sm" aria-label={t("tax.cities")} value={r._cities ?? ""} onChange={(e) => set(i, { _cities: e.target.value })} placeholder="*" />
                </td>
                <td>
                  <input className="input input-sm w-sm" aria-label={t("tax.rate")} type="number" step="0.0001" value={r.rate} onChange={(e) => set(i, { rate: e.target.value })} />
                </td>
                <td>
                  <input className="input input-sm" aria-label={t("tax.name")} value={r.name} onChange={(e) => set(i, { name: e.target.value })} />
                </td>
                <td>
                  <input className="input input-sm w-xs" aria-label={t("tax.priority")} type="number" min="1" value={r.priority} onChange={(e) => set(i, { priority: e.target.value })} />
                </td>
                <td>
                  <input type="checkbox" aria-label={t("tax.compound")} checked={!!r.compound} onChange={(e) => set(i, { compound: e.target.checked })} />
                </td>
                <td>
                  <input type="checkbox" aria-label={t("tax.shipping")} checked={r.shipping !== false} onChange={(e) => set(i, { shipping: e.target.checked })} />
                </td>
                <td>
                  <button
                    type="button"
                    className="icon-btn danger"
                    aria-label={t("common.remove")}
                    onClick={() => {
                      if (!r._new) setRemoved((x) => [...x, r.id]);
                      setRows((rs) => rs.filter((_, j) => j !== i));
                    }}
                  >
                    <FiTrash2 />
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {rows.length === 0 && <p className="muted small">{t("tax.noRates")}</p>}
      </div>
      <p className="hint">{t("tax.hint")}</p>
      <div className="row">
        <button type="button" className="btn btn-secondary btn-sm" onClick={() => setRows((rs) => [...rs, blankRate(cls)])}>
          <FiPlus aria-hidden="true" /> {t("tax.addRate")}
        </button>
        <button type="button" className="btn btn-primary btn-sm" onClick={saveAll} disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </div>
  );
};

const TaxTab = () => {
  const { t, errMsg } = useI18n();
  const { data: classes, reload } = useAsync(async () => asArray((await api.get("/v1/admin/tax-classes")).data), [], []);
  const [active, setActive] = useState("");
  const [name, setName] = useState("");
  const [deleting, setDeleting] = useState(null);
  const others = classes.filter((c) => c.slug && c.slug !== "standard");

  const addClass = async (e) => {
    e.preventDefault();
    try {
      await api.post("/v1/admin/tax-classes", { name: name.trim() });
      setName("");
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className="stack-lg">
      <section className="card stack">
        <h2>{t("tax.classes")}</h2>
        <p className="muted small">{t("tax.classesHint")}</p>
        <ul className="plain-list">
          <li className="list-row">
            <span>{t("tax.standard")}</span>
          </li>
          {others.map((c) => (
            <li key={c.slug} className="list-row">
              <span>{c.name}</span>
              <button type="button" className="icon-btn danger" onClick={() => setDeleting(c)} aria-label={t("common.delete")}>
                <FiTrash2 />
              </button>
            </li>
          ))}
        </ul>
        <form className="row" onSubmit={addClass}>
          <label className="sr-only" htmlFor="tax-class-name">
            {t("tax.newClass")}
          </label>
          <input id="tax-class-name" className="input input-sm" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("tax.newClass")} />
          <button type="submit" className="btn btn-secondary btn-sm" disabled={!name.trim()}>
            {t("common.add")}
          </button>
        </form>
      </section>
      <section className="card stack">
        <h2>{t("tax.rates")}</h2>
        <Tabs tabs={[{ id: "", label: t("tax.standard") }, ...others.map((c) => ({ id: c.slug, label: c.name }))]} active={active} onChange={setActive} />
        <RatesTable cls={active} key={active} />
      </section>
      <ConfirmModal
        show={!!deleting}
        title={t("tax.deleteClass")}
        body={deleting ? t("tax.deleteClassBody", { name: deleting.name }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          const c = deleting;
          setDeleting(null);
          try {
            await api.delete(`/v1/admin/tax-classes/${c.slug}`);
            if (active === c.slug) setActive("");
            reload();
          } catch (err) {
            toast.error(errMsg(err));
          }
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default TaxTab;
