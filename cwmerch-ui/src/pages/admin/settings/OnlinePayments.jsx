import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { FiCheckCircle, FiCopy, FiXCircle } from "react-icons/fi";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Field, SelectField, Spinner, TextArea, TextField, Toggle } from "../../../components/ui";
import PaymentLogo from "../../../components/PaymentLogo";

// Known provider fields; anything else the API sends is still editable (generic).
const PROVIDERS = {
  stripe: {
    modes: ["test", "live"],
    settings: [{ key: "publishableKey", type: "text" }],
    secrets: ["secretKey", "webhookSecret"],
  },
  konnect: {
    modes: ["test", "live"],
    settings: [
      { key: "walletId", type: "text" },
      { key: "acceptedPaymentMethods", type: "multi", options: ["wallet", "bank_card", "e-DINAR"] },
      { key: "lifespan", type: "number" },
      { key: "addPaymentFeesToAmount", type: "bool" },
      { key: "theme", type: "select", options: ["light", "dark"] },
    ],
    secrets: ["apiKey"],
  },
  flouci: {
    modes: ["test", "live"],
    settings: [
      { key: "publicKey", type: "text" },
      { key: "acceptCard", type: "bool" },
      { key: "sessionTimeoutSecs", type: "number" },
    ],
    secrets: ["privateKey"],
  },
};

const label = (t, key) => {
  const k = `onlinePayments.fields.${key}`;
  const v = t(k);
  return v === k ? key : v;
};

const CopyField = ({ label: text, value }) => {
  const { t } = useI18n();
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      toast.success(t("onlinePayments.copied"));
    } catch {
      toast.error(t("onlinePayments.copyFailed"));
    }
  };
  return (
    <Field label={text}>
      {(id) => (
        <div className="input-group">
          <input id={id} className="input mono" readOnly value={value || ""} onFocus={(e) => e.target.select()} />
          <button type="button" className="btn btn-secondary" onClick={copy} disabled={!value} aria-label={t("onlinePayments.copy")} title={t("onlinePayments.copy")}>
            <FiCopy aria-hidden="true" />
          </button>
        </div>
      )}
    </Field>
  );
};

const Provider = ({ provider, onSaved }) => {
  const { t, errMsg } = useI18n();
  const spec = PROVIDERS[provider.id] || { modes: ["test", "live"], settings: [], secrets: [] };
  const [draft, setDraft] = useState(null);
  const [secrets, setSecrets] = useState({});
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [test, setTest] = useState(null);

  useEffect(() => {
    setDraft({
      enabled: !!provider.enabled,
      mode: provider.mode || "test",
      title: provider.title || "",
      description: provider.description || "",
      settings: { ...(provider.settings || {}) },
    });
    setSecrets({});
  }, [provider]);

  if (!draft) return null;
  const knownKeys = spec.settings.map((f) => f.key);
  const extraKeys = Object.keys(draft.settings).filter((k) => !knownKeys.includes(k) && typeof draft.settings[k] !== "object");
  const secretKeys = [...new Set([...spec.secrets, ...Object.keys(provider.secrets || {})])];
  const setSetting = (k, v) => setDraft((d) => ({ ...d, settings: { ...d.settings, [k]: v } }));

  const save = async (override = {}) => {
    setBusy(true);
    try {
      // Blank secrets are left out: the API keeps the stored value.
      const filled = Object.fromEntries(Object.entries(secrets).filter(([, v]) => v && v.trim()));
      await api.put(`/v1/admin/payment-providers/${provider.id}`, { ...draft, ...override, secrets: filled });
      toast.success(t("admin.saved"));
      setSecrets({});
      onSaved();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const runTest = async () => {
    setTest({ running: true });
    try {
      const { data } = await api.post(`/v1/admin/payment-providers/${provider.id}/test`, {});
      setTest({ ok: !!data?.ok, message: data?.message || "" });
    } catch (err) {
      setTest({ ok: false, message: errMsg(err) });
    }
  };

  const renderSetting = (f) => {
    const v = draft.settings[f.key];
    switch (f.type) {
      case "bool":
        return <Toggle key={f.key} checked={!!v} onChange={(on) => setSetting(f.key, on)} label={label(t, f.key)} />;
      case "number":
        return <TextField key={f.key} type="number" label={label(t, f.key)} value={v ?? ""} onChange={(x) => setSetting(f.key, x === "" ? null : Number(x))} />;
      case "select":
        return <SelectField key={f.key} label={label(t, f.key)} value={v || ""} onChange={(x) => setSetting(f.key, x)} options={f.options.map((o) => ({ value: o, label: o }))} placeholder={t("settings.choose")} />;
      case "multi": {
        const list = Array.isArray(v) ? v : [];
        return (
          <fieldset key={f.key} className="fieldset">
            <legend className="field-label">{label(t, f.key)}</legend>
            <div className="row wrap">
              {f.options.map((o) => (
                <label key={o} className="radio-row">
                  <input type="checkbox" checked={list.includes(o)} onChange={(e) => setSetting(f.key, e.target.checked ? [...list, o] : list.filter((x) => x !== o))} />
                  <span>{o}</span>
                </label>
              ))}
            </div>
          </fieldset>
        );
      }
      default:
        return <TextField key={f.key} label={label(t, f.key)} value={v ?? ""} onChange={(x) => setSetting(f.key, x)} className="mono-field" />;
    }
  };

  const modeLabel = (m) => t(`onlinePayments.modes.${provider.id === "konnect" ? `konnect_${m}` : m}`);

  return (
    <section className="card stack">
      <div className="section-head">
        <div>
          <h3 className="provider-title">
            <PaymentLogo id={provider.id} />
            {provider.name || provider.id}{" "}
            <span className={`badge ${provider.configured ? "badge-completed" : "badge-pending"}`}>{provider.configured ? t("onlinePayments.configured") : t("onlinePayments.notConfigured")}</span>{" "}
            <span className="badge">{modeLabel(draft.mode)}</span>
          </h3>
          <p className="muted small">{t(`onlinePayments.help.${provider.id}`) !== `onlinePayments.help.${provider.id}` ? t(`onlinePayments.help.${provider.id}`) : provider.description}</p>
          {Array.isArray(provider.currencies) && provider.currencies.length > 0 && !provider.currencies.includes("*") && (
            <p className="muted small">{t("onlinePayments.currencies", { list: provider.currencies.join(", ") })}</p>
          )}
        </div>
        <div className="row">
          <Toggle
            checked={draft.enabled}
            disabled={busy}
            onChange={(on) => {
              setDraft((d) => ({ ...d, enabled: on }));
              save({ enabled: on });
            }}
            label={draft.enabled ? t("payments.enabled") : t("payments.disabled")}
          />
          <button type="button" className="btn btn-secondary btn-sm" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
            {open ? t("common.close") : t("payments.manage")}
          </button>
        </div>
      </div>
      {open && (
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault();
            save();
          }}
        >
          <div className="form-grid">
            <SelectField label={t("onlinePayments.mode")} value={draft.mode} onChange={(mode) => setDraft((d) => ({ ...d, mode }))} options={spec.modes.map((m) => ({ value: m, label: modeLabel(m) }))} />
            <TextField label={t("payments.fields.title")} value={draft.title} onChange={(title) => setDraft((d) => ({ ...d, title }))} />
          </div>
          <TextArea label={t("payments.fields.description")} value={draft.description} onChange={(description) => setDraft((d) => ({ ...d, description }))} rows={2} />
          <fieldset className="fieldset stack">
            <legend>{t("onlinePayments.settings")}</legend>
            {spec.settings.map(renderSetting)}
            {extraKeys.map((k) => renderSetting({ key: k, type: typeof draft.settings[k] === "boolean" ? "bool" : typeof draft.settings[k] === "number" ? "number" : "text" }))}
          </fieldset>
          <fieldset className="fieldset stack">
            <legend>{t("onlinePayments.secrets")}</legend>
            {secretKeys.map((k) => {
              const stored = !!provider.secrets?.[k];
              return (
                <TextField
                  key={k}
                  type="password"
                  autoComplete="new-password"
                  label={label(t, k)}
                  value={secrets[k] || ""}
                  onChange={(v) => setSecrets((s) => ({ ...s, [k]: v }))}
                  placeholder={stored ? "••••••••" : ""}
                  hint={stored ? t("onlinePayments.secretStored") : t("onlinePayments.secretNotSet")}
                />
              );
            })}
          </fieldset>
          <CopyField label={t("onlinePayments.webhookUrl")} value={provider.webhookUrl} />
          <p className="hint">{t(`onlinePayments.webhookHint.${provider.id}`) !== `onlinePayments.webhookHint.${provider.id}` ? t(`onlinePayments.webhookHint.${provider.id}`) : t("onlinePayments.webhookHintGeneric")}</p>
          {test && !test.running && (
            <p className={`notice ${test.ok ? "notice-success" : "notice-error"}`} role="status">
              {test.ok ? <FiCheckCircle aria-hidden="true" /> : <FiXCircle aria-hidden="true" />} {test.message || (test.ok ? t("onlinePayments.testOk") : t("onlinePayments.testFailed"))}
            </p>
          )}
          <div className="row wrap">
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {t("common.saveChanges")}
            </button>
            <button type="button" className="btn btn-secondary" onClick={runTest} disabled={test?.running}>
              {test?.running ? t("onlinePayments.testing") : t("onlinePayments.test")}
            </button>
          </div>
          <p className="hint">{t("onlinePayments.testHint")}</p>
        </form>
      )}
    </section>
  );
};

// OnlinePayments: the online gateway adapters (GET /v1/admin/payment-providers).
const OnlinePayments = ({ onSaved }) => {
  const { t } = useI18n();
  const { data, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/payment-providers")).data), [], []);
  return (
    <section className="stack-lg">
      <div>
        <h2>{t("onlinePayments.title")}</h2>
        <p className="muted small">{t("onlinePayments.intro")}</p>
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        data.map((p) => (
          <Provider
            key={p.id}
            provider={p}
            onSaved={() => {
              reload();
              onSaved?.();
            }}
          />
        ))
      )}
    </section>
  );
};

export default OnlinePayments;
