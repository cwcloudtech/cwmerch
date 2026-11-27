import { useMemo, useState } from "react";
import { toast } from "react-toastify";
import { FiEdit2, FiPlus, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { useI18n } from "../../../i18n/I18nContext";
import { ConfirmModal, ErrorBox, Modal, SelectField, Spinner, TextArea, TextField } from "../../../components/ui";
import { useAllCountries } from "../useAllCountries";

export const TAX_MODES = ["vat", "stamp", "vat_stamp", "none"];

// GET /v1/countries carries no currency: a small ISO country → currency map
// pre-fills the "add a country" form (editable; blank when unknown).
const EURO = "AT BE CY DE EE ES FI FR GR HR IE IT LT LU LV MC MT NL PT SI SK SM VA AD ME XK".split(" ");
const CURRENCIES = {
  ...Object.fromEntries(EURO.map((c) => [c, "EUR"])),
  AE: "AED", AU: "AUD", BR: "BRL", CA: "CAD", CH: "CHF", CN: "CNY", CZ: "CZK", DK: "DKK", DZ: "DZD", EG: "EGP",
  GB: "GBP", HU: "HUF", IN: "INR", JP: "JPY", KW: "KWD", LB: "LBP", LI: "CHF", LY: "LYD", MA: "MAD", MR: "MRU",
  MX: "MXN", NG: "NGN", NO: "NOK", NZ: "NZD", PL: "PLN", QA: "QAR", RO: "RON", RU: "RUB", SA: "SAR", SE: "SEK",
  TN: "TND", TR: "TRY", US: "USD", ZA: "ZAR",
  BJ: "XOF", BF: "XOF", CI: "XOF", GW: "XOF", ML: "XOF", NE: "XOF", SN: "XOF", TG: "XOF",
  CM: "XAF", CF: "XAF", TD: "XAF", CG: "XAF", GQ: "XAF", GA: "XAF",
};

const EMPTY_ROW = {
  name: "",
  taxMode: "vat",
  vatLabel: "",
  defaultVatRate: 0,
  stampAmount: 0,
  stampLabel: "",
  currency: "",
  registrationLabel: "",
  vatNumberLabel: "",
  numberPrefix: "FAC-",
  locale: "fr",
  vatExemptionMention: "",
  legalMentions: "",
};

// flag renders a regional-indicator emoji for an ISO alpha-2 code.
export const flag = (code) =>
  /^[A-Z]{2}$/.test(code || "") ? String.fromCodePoint(...[...code].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65)) : "🌐";

const RowEditor = ({ row, isNew, available, onClose, onSaved }) => {
  const { t, errMsg } = useI18n();
  const [draft, setDraft] = useState(() => ({ ...EMPTY_ROW, ...row }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const set = (k) => (v) => setDraft((d) => ({ ...d, [k]: v }));
  const iso = draft.country;

  const pickCountry = (code) => {
    const c = available.find((x) => x.code === code);
    setDraft((d) => ({ ...d, country: code, name: c?.name || d.name, currency: CURRENCIES[code] || d.currency }));
  };

  const submit = async (e) => {
    e.preventDefault();
    if (!iso) return;
    setBusy(true);
    setError(null);
    try {
      const { country: _c, fallback: _f, ...body } = draft;
      await api.put(`/v1/admin/invoicing/country-defaults/${encodeURIComponent(iso)}`, {
        ...body,
        defaultVatRate: Number(body.defaultVatRate) || 0,
        stampAmount: Number(body.stampAmount) || 0,
      });
      toast.success(t("admin.saved"));
      onSaved();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal show title={isNew ? t("countryDefaults.add") : t("countryDefaults.edit", { name: draft.name || iso })} onClose={onClose} wide>
      <form className="stack" onSubmit={submit}>
        {isNew ? (
          <SelectField
            label={t("countryDefaults.country")}
            value={iso || ""}
            onChange={pickCountry}
            options={available.map((c) => ({ value: c.code, label: `${flag(c.code)} ${c.name} (${c.code})` }))}
            placeholder={t("settings.choose")}
            required
          />
        ) : (
          <p className="muted">
            {flag(iso)} <strong>{iso === "*" ? t("countryDefaults.fallback") : iso}</strong>
          </p>
        )}
        <div className="form-grid">
          <TextField label={t("countryDefaults.name")} value={draft.name} onChange={set("name")} required />
          <SelectField label={t("invoicing.fields.taxMode")} value={draft.taxMode} onChange={set("taxMode")} options={TAX_MODES.map((v) => ({ value: v, label: t(`invoicing.taxModes.${v}`) }))} />
          <TextField label={t("invoicing.fields.vatLabel")} value={draft.vatLabel} onChange={set("vatLabel")} />
          <TextField label={t("invoicing.fields.defaultVatRate")} type="number" step="0.01" min="0" value={draft.defaultVatRate} onChange={set("defaultVatRate")} />
          <TextField label={t("invoicing.fields.stampAmount")} type="number" step="0.001" min="0" value={draft.stampAmount} onChange={set("stampAmount")} />
          <TextField label={t("invoicing.fields.stampLabel")} value={draft.stampLabel} onChange={set("stampLabel")} />
          <TextField label={t("invoicing.fields.currency")} value={draft.currency} onChange={(v) => set("currency")(v.toUpperCase())} maxLength={3} />
          <TextField label={t("invoicing.fields.locale")} value={draft.locale} onChange={set("locale")} />
          <TextField label={t("invoicing.fields.registrationLabel")} value={draft.registrationLabel} onChange={set("registrationLabel")} />
          <TextField label={t("invoicing.fields.vatNumberLabel")} value={draft.vatNumberLabel} onChange={set("vatNumberLabel")} />
          <TextField label={t("invoicing.fields.numberPrefix")} value={draft.numberPrefix} onChange={set("numberPrefix")} />
        </div>
        <TextArea label={t("invoicing.fields.vatExemptionMention")} value={draft.vatExemptionMention} onChange={set("vatExemptionMention")} rows={2} />
        <TextArea label={t("invoicing.fields.legalMentions")} value={draft.legalMentions} onChange={set("legalMentions")} rows={4} />
        <ErrorBox error={error} />
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy || !iso}>
            {t("common.save")}
          </button>
        </div>
      </form>
    </Modal>
  );
};

// CountryDefaults administers the invoicing decision table
// (GET/PUT/DELETE /v1/admin/invoicing/country-defaults[/{iso}]).
const CountryDefaults = () => {
  const { t, errMsg } = useI18n();
  const countries = useAllCountries();
  const { data: rows, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/invoicing/country-defaults")).data), [], []);
  const [editing, setEditing] = useState(null);
  const [deleting, setDeleting] = useState(null);

  const sorted = useMemo(() => [...rows].sort((a, b) => (a.country === "*" ? 1 : b.country === "*" ? -1 : String(a.name || a.country).localeCompare(String(b.name || b.country)))), [rows]);
  const available = useMemo(() => countries.filter((c) => !rows.some((r) => r.country === c.code)), [countries, rows]);

  const remove = async () => {
    const iso = deleting;
    setDeleting(null);
    try {
      await api.delete(`/v1/admin/invoicing/country-defaults/${encodeURIComponent(iso)}`);
      toast.success(t("admin.deleted"));
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <section className="card stack">
      <div className="section-head">
        <div>
          <h2>{t("countryDefaults.title")}</h2>
          <p className="muted small">{t("countryDefaults.intro")}</p>
        </div>
        <button type="button" className="btn btn-primary btn-sm" onClick={() => setEditing({ row: { country: "" }, isNew: true })} disabled={available.length === 0}>
          <FiPlus aria-hidden="true" /> {t("countryDefaults.add")}
        </button>
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{t("countryDefaults.country")}</th>
                <th scope="col">{t("countryDefaults.name")}</th>
                <th scope="col">{t("invoicing.fields.taxMode")}</th>
                <th scope="col" className="num">
                  {t("countryDefaults.vatRate")}
                </th>
                <th scope="col" className="num">
                  {t("countryDefaults.stamp")}
                </th>
                <th scope="col">{t("invoicing.fields.currency")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((r) => (
                <tr key={r.country}>
                  <td>
                    <span aria-hidden="true">{flag(r.country)}</span> <code>{r.country}</code>
                  </td>
                  <td>{r.country === "*" ? t("countryDefaults.fallback") : r.name}</td>
                  <td>{t(`invoicing.taxModes.${r.taxMode}`)}</td>
                  <td className="num">{r.taxMode === "vat" || r.taxMode === "vat_stamp" ? `${Number(r.defaultVatRate || 0)} %` : "—"}</td>
                  <td className="num">{r.taxMode === "stamp" || r.taxMode === "vat_stamp" ? `${Number(r.stampAmount || 0)}${r.stampLabel ? ` (${r.stampLabel})` : ""}` : "—"}</td>
                  <td>{r.currency}</td>
                  <td>
                    <div className="row-actions">
                      <button type="button" className="icon-btn" onClick={() => setEditing({ row: r, isNew: false })} aria-label={t("common.edit")} title={t("common.edit")}>
                        <FiEdit2 />
                      </button>
                      {r.country !== "*" && (
                        <button type="button" className="icon-btn danger" onClick={() => setDeleting(r.country)} aria-label={t("common.delete")} title={t("common.delete")}>
                          <FiTrash2 />
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      {editing && (
        <RowEditor
          row={editing.row}
          isNew={editing.isNew}
          available={available}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            reload();
          }}
        />
      )}
      <ConfirmModal
        show={!!deleting}
        title={t("countryDefaults.deleteTitle")}
        body={t("countryDefaults.deleteBody", { country: deleting || "" })}
        confirmLabel={t("common.delete")}
        onConfirm={remove}
        onCancel={() => setDeleting(null)}
      />
    </section>
  );
};

export default CountryDefaults;
