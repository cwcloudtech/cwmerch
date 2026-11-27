import { useEffect, useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiEye, FiFileText } from "react-icons/fi";
import { api, asArray, openBlobInTab } from "../../../api/client";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, SelectField, Spinner, Tabs, TextArea, TextField, Toggle } from "../../../components/ui";
import { useAllCountries } from "../useAllCountries";
import CountryDefaults, { TAX_MODES } from "./CountryDefaults";

const AUTO_GENERATE = ["none", "processing", "completed"];
const LOCALES = ["fr", "en"];
const SELLER_FIELDS = [
  ["name", "text"],
  ["address", "textarea"],
  ["postcode", "text"],
  ["city", "text"],
  ["country", "country"],
  ["email", "email"],
  ["phone", "tel"],
  ["website", "url"],
  ["registrationLabel", "text"],
  ["registrationNumber", "text"],
  ["vatNumberLabel", "text"],
  ["vatNumber", "text"],
  ["iban", "text"],
  ["bic", "text"],
];

const EMPTY = {
  enabled: false,
  autoGenerate: "none",
  numberPrefix: "FAC-",
  nextNumber: 1,
  numberPadding: 5,
  resetYearly: true,
  country: "",
  locale: "fr",
  taxMode: "vat",
  vatLabel: "",
  defaultVatRate: 0,
  vatExemptionMention: "",
  stampAmount: 0,
  stampLabel: "",
  stampChargeCustomer: false,
  seller: {},
  legalMentions: "",
  paymentTerms: "",
  footer: "",
  template: "",
};

const num = (v) => (v === "" || v === null || v === undefined ? 0 : Number(v));

// DefaultsPreview shows what "apply country defaults" will write.
const DefaultsPreview = ({ row }) => {
  const { t } = useI18n();
  if (!row) return null;
  const items = [
    ["taxMode", t(`invoicing.taxModes.${row.taxMode}`)],
    ["vatLabel", row.vatLabel],
    ["defaultVatRate", row.defaultVatRate !== undefined ? `${row.defaultVatRate} %` : ""],
    ["stampAmount", row.stampAmount ? `${row.stampAmount} ${row.currency || ""}` : ""],
    ["stampLabel", row.stampLabel],
    ["currency", row.currency],
    ["registrationLabel", row.registrationLabel],
    ["vatNumberLabel", row.vatNumberLabel],
    ["numberPrefix", row.numberPrefix],
    ["locale", row.locale],
    ["vatExemptionMention", row.vatExemptionMention],
    ["legalMentions", row.legalMentions],
  ].filter(([, v]) => v !== undefined && v !== null && v !== "");
  return (
    <div className="defaults-preview">
      <p className="small muted">
        {row.country === "*" || row.fallback ? t("invoicing.previewFallback") : t("invoicing.previewFor", { name: row.name || row.country })}
      </p>
      <dl className="kv-list">
        {items.map(([k, v]) => (
          <div key={k}>
            <dt>{t(`invoicing.fields.${k}`)}</dt>
            <dd className="pre-line">{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  );
};

// TemplateEditor: the Markdown / Go-template of the invoice (empty = default).
const TemplateEditor = ({ value, onChange }) => {
  const { t, errMsg } = useI18n();
  const [meta, setMeta] = useState(null);
  const [orderId, setOrderId] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .get("/v1/admin/invoicing/template")
      .then(({ data }) => setMeta(data || {}))
      .catch(() => setMeta({}));
  }, []);

  const loadDefault = async () => {
    try {
      const data = meta?.default ? meta : (await api.get("/v1/admin/invoicing/template")).data;
      setMeta(data);
      if (value && !window.confirm(t("invoicing.replaceTemplate"))) return;
      onChange(data?.default || "");
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const preview = async () => {
    setBusy(true);
    try {
      const body = { ...(value ? { template: value } : {}), ...(Number(orderId) > 0 ? { orderId: Number(orderId) } : {}) };
      await openBlobInTab("post", "/v1/admin/invoicing/preview", { data: body, fallbackName: "invoice-preview.pdf" });
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const variables = asArray(meta?.variables);

  return (
    <section className="card stack settings-section">
      <h2>{t("invoicing.template")}</h2>
      <p className="muted small">{t("invoicing.templateIntro")}</p>
      <TextArea label={t("invoicing.templateLabel")} value={value} onChange={onChange} rows={18} className="mono-area" placeholder={t("invoicing.templateEmpty")} spellCheck={false} dir="ltr" />
      <div className="row wrap align-end">
        <button type="button" className="btn btn-secondary" onClick={loadDefault}>
          <FiFileText aria-hidden="true" /> {t("invoicing.loadDefault")}
        </button>
        <TextField label={t("invoicing.previewOrder")} type="number" min="1" value={orderId} onChange={setOrderId} className="w-md" placeholder={t("invoicing.previewOrderPlaceholder")} />
        <button type="button" className="btn btn-secondary" onClick={preview} disabled={busy}>
          <FiEye aria-hidden="true" /> {busy ? t("common.loading") : t("invoicing.previewPdf")}
        </button>
      </div>
      {variables.length > 0 && (
        <details className="variables">
          <summary>{t("invoicing.variables", { count: variables.length })}</summary>
          <div className="table-wrap">
            <table className="table">
              <tbody>
                {variables.map((v) => (
                  <tr key={v.name}>
                    <td>
                      <code>{`{{ ${v.name} }}`}</code>
                    </td>
                    <td className="small">{v.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </section>
  );
};

const InvoiceSettingsForm = () => {
  const { t, errMsg } = useI18n();
  const countries = useAllCountries();
  const [s, setS] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [defaults, setDefaults] = useState([]);
  const [applyCountry, setApplyCountry] = useState("");
  const [createTaxRate, setCreateTaxRate] = useState(false);

  useEffect(() => {
    api
      .get("/v1/admin/invoicing/settings")
      .then(({ data }) => {
        const next = { ...EMPTY, ...data, seller: { ...(data?.seller || {}) } };
        setS(next);
        setApplyCountry(next.country || "");
      })
      .catch(setError);
    api
      .get("/v1/admin/invoicing/country-defaults")
      .then(({ data }) => setDefaults(asArray(data)))
      .catch(() => {});
  }, []);

  const countryOptions = useMemo(() => countries.map((c) => ({ value: c.code, label: `${c.name} (${c.code})` })), [countries]);
  const previewRow = useMemo(() => {
    if (!applyCountry) return null;
    return defaults.find((d) => d.country === applyCountry) || (defaults.find((d) => d.country === "*") ? { ...defaults.find((d) => d.country === "*"), fallback: true } : null);
  }, [applyCountry, defaults]);

  if (!s) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setS((x) => ({ ...x, [k]: v }));
  const setSeller = (k) => (v) => setS((x) => ({ ...x, seller: { ...x.seller, [k]: v } }));
  const vat = s.taxMode === "vat" || s.taxMode === "vat_stamp";
  const stamp = s.taxMode === "stamp" || s.taxMode === "vat_stamp";

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const body = {
        ...s,
        nextNumber: num(s.nextNumber),
        numberPadding: num(s.numberPadding),
        defaultVatRate: num(s.defaultVatRate),
        stampAmount: num(s.stampAmount),
      };
      const { data } = await api.put("/v1/admin/invoicing/settings", body);
      if (data && typeof data === "object") setS({ ...EMPTY, ...data, seller: { ...(data.seller || {}) } });
      toast.success(t("admin.saved"));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const applyDefaults = async () => {
    if (!applyCountry) return;
    setBusy(true);
    try {
      const { data } = await api.post("/v1/admin/invoicing/apply-defaults", { country: applyCountry, createTaxRate });
      if (data && typeof data === "object") setS({ ...EMPTY, ...data, seller: { ...(data.seller || {}) } });
      toast.success(t("invoicing.defaultsApplied"));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="stack-lg" onSubmit={save}>
      <section className="card stack settings-section">
        <h2>{t("invoicing.general")}</h2>
        <Toggle checked={s.enabled} onChange={set("enabled")} label={t("invoicing.fields.enabled")} hint={t("invoicing.enabledHint")} />
        <div className="form-grid">
          <SelectField
            label={t("invoicing.fields.autoGenerate")}
            value={s.autoGenerate}
            onChange={set("autoGenerate")}
            options={AUTO_GENERATE.map((v) => ({ value: v, label: t(`invoicing.autoGenerate.${v}`) }))}
          />
          <SelectField label={t("invoicing.fields.locale")} value={s.locale} onChange={set("locale")} options={LOCALES.map((v) => ({ value: v, label: v }))} />
        </div>
      </section>

      <section className="card stack settings-section">
        <h2>{t("invoicing.numbering")}</h2>
        <div className="form-grid-3">
          <TextField label={t("invoicing.fields.numberPrefix")} value={s.numberPrefix} onChange={set("numberPrefix")} />
          <TextField label={t("invoicing.fields.nextNumber")} type="number" min="1" value={s.nextNumber} onChange={set("nextNumber")} />
          <TextField label={t("invoicing.fields.numberPadding")} type="number" min="0" max="12" value={s.numberPadding} onChange={set("numberPadding")} />
        </div>
        <Toggle checked={s.resetYearly} onChange={set("resetYearly")} label={t("invoicing.fields.resetYearly")} />
        <p className="hint">
          {t("invoicing.numberExample", {
            example: `${s.numberPrefix || ""}${s.resetYearly ? `${new Date().getFullYear()}-` : ""}${String(num(s.nextNumber) || 1).padStart(num(s.numberPadding), "0")}`,
          })}
        </p>
      </section>

      <section className="card stack settings-section">
        <h2>{t("invoicing.countryAndTax")}</h2>
        <div className="form-grid">
          <SelectField label={t("invoicing.fields.country")} value={s.country} onChange={set("country")} options={countryOptions} placeholder={t("settings.choose")} />
          <SelectField label={t("invoicing.fields.taxMode")} value={s.taxMode} onChange={set("taxMode")} options={TAX_MODES.map((v) => ({ value: v, label: t(`invoicing.taxModes.${v}`) }))} />
        </div>
        <fieldset className="fieldset stack">
          <legend>{t("invoicing.applyDefaults")}</legend>
          <p className="hint">{t("invoicing.applyDefaultsHint")}</p>
          <div className="row wrap align-end">
            <SelectField label={t("invoicing.fields.country")} value={applyCountry} onChange={setApplyCountry} options={countryOptions} placeholder={t("settings.choose")} />
            <Toggle checked={createTaxRate} onChange={setCreateTaxRate} label={t("invoicing.createTaxRate")} />
            <button type="button" className="btn btn-secondary" onClick={applyDefaults} disabled={busy || !applyCountry}>
              {t("invoicing.applyDefaults")}
            </button>
          </div>
          <DefaultsPreview row={previewRow} />
        </fieldset>
        {vat && (
          <div className="form-grid-3">
            <TextField label={t("invoicing.fields.vatLabel")} value={s.vatLabel} onChange={set("vatLabel")} />
            <TextField label={t("invoicing.fields.defaultVatRate")} type="number" step="0.01" min="0" value={s.defaultVatRate} onChange={set("defaultVatRate")} />
          </div>
        )}
        <TextField label={t("invoicing.fields.vatExemptionMention")} hint={t("invoicing.vatExemptionHint")} value={s.vatExemptionMention} onChange={set("vatExemptionMention")} />
        {stamp && (
          <>
            <div className="form-grid-3">
              <TextField label={t("invoicing.fields.stampAmount")} type="number" step="0.001" min="0" value={s.stampAmount} onChange={set("stampAmount")} />
              <TextField label={t("invoicing.fields.stampLabel")} value={s.stampLabel} onChange={set("stampLabel")} />
            </div>
            <Toggle checked={s.stampChargeCustomer} onChange={set("stampChargeCustomer")} label={t("invoicing.fields.stampChargeCustomer")} hint={t("invoicing.stampChargeHint")} />
          </>
        )}
      </section>

      <section className="card stack settings-section">
        <h2>{t("invoicing.seller")}</h2>
        <div className="form-grid">
          {SELLER_FIELDS.map(([k, type]) =>
            type === "textarea" ? (
              <TextArea key={k} label={t(`invoicing.sellerFields.${k}`)} value={s.seller[k]} onChange={setSeller(k)} rows={2} className="span-2" />
            ) : type === "country" ? (
              <SelectField key={k} label={t(`invoicing.sellerFields.${k}`)} value={s.seller[k]} onChange={setSeller(k)} options={countryOptions} placeholder={t("settings.choose")} />
            ) : (
              <TextField key={k} type={type} label={t(`invoicing.sellerFields.${k}`)} value={s.seller[k]} onChange={setSeller(k)} />
            )
          )}
        </div>
      </section>

      <section className="card stack settings-section">
        <h2>{t("invoicing.texts")}</h2>
        <TextArea label={t("invoicing.fields.legalMentions")} value={s.legalMentions} onChange={set("legalMentions")} rows={3} />
        <TextArea label={t("invoicing.fields.paymentTerms")} value={s.paymentTerms} onChange={set("paymentTerms")} rows={2} />
        <TextArea label={t("invoicing.fields.footer")} value={s.footer} onChange={set("footer")} rows={2} />
      </section>

      <TemplateEditor value={s.template} onChange={set("template")} />

      <div className="sticky-save">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

// InvoicingTab: invoice settings + template, and the per-country decision table.
const InvoicingTab = () => {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const view = params.get("view") === "countries" ? "countries" : "settings";
  return (
    <div className="stack-lg">
      <Tabs
        tabs={[
          { id: "settings", label: t("invoicing.tabs.settings") },
          { id: "countries", label: t("invoicing.tabs.countries") },
        ]}
        active={view}
        onChange={(v) => setParams(v === "countries" ? { view: v } : {})}
      />
      {view === "settings" ? <InvoiceSettingsForm /> : <CountryDefaults />}
    </div>
  );
};

export default InvoicingTab;
