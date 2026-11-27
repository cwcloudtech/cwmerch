import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { FiPlus, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { useShop } from "../../../context/ShopContext";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Spinner, TextArea, TextField, Toggle } from "../../../components/ui";
import OnlinePayments from "./OnlinePayments";
import { ONLINE_PROVIDERS } from "../../../lib/payment";

const ACCOUNT_FIELDS = ["accountName", "accountNumber", "bankName", "sortCode", "iban", "bic"];
const LONG = ["description", "instructions"];
const isYesNo = (v) => v === "yes" || v === "no";

const Gateway = ({ gateway, onSaved }) => {
  const { t, errMsg } = useI18n();
  const [enabled, setEnabled] = useState(!!gateway.enabled);
  const [settings, setSettings] = useState({ ...(gateway.settings || {}) });
  const [accounts, setAccounts] = useState(asArray(gateway.accounts ?? gateway.settings?.accounts));
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    setEnabled(!!gateway.enabled);
    setSettings({ ...(gateway.settings || {}) });
    setAccounts(asArray(gateway.accounts ?? gateway.settings?.accounts));
  }, [gateway]);

  const save = async (next = {}) => {
    setBusy(true);
    try {
      const { accounts: _a, ...rest } = settings;
      const body = { enabled, settings: rest, ...(gateway.id === "bacs" ? { accounts } : {}), ...next };
      await api.put(`/v1/admin/payment-gateways/${gateway.id}`, body);
      toast.success(t("admin.saved"));
      onSaved();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const keys = Object.keys(settings).filter((k) => k !== "accounts" && k !== "enabled" && typeof settings[k] !== "object");

  return (
    <section className="card stack">
      <div className="section-head">
        <div>
          <h2>{gateway.title || gateway.id}</h2>
          <p className="muted small">{gateway.description}</p>
        </div>
        <div className="row">
          <Toggle
            checked={enabled}
            disabled={busy}
            onChange={(on) => {
              setEnabled(on);
              save({ enabled: on });
            }}
            label={enabled ? t("payments.enabled") : t("payments.disabled")}
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
          {keys.map((k) =>
            isYesNo(settings[k]) ? (
              <Toggle key={k} checked={settings[k] === "yes"} onChange={(on) => setSettings({ ...settings, [k]: on ? "yes" : "no" })} label={t(`payments.fields.${k}`) !== `payments.fields.${k}` ? t(`payments.fields.${k}`) : k} />
            ) : LONG.includes(k) ? (
              <TextArea key={k} label={t(`payments.fields.${k}`)} value={settings[k]} onChange={(v) => setSettings({ ...settings, [k]: v })} rows={3} />
            ) : (
              <TextField key={k} label={t(`payments.fields.${k}`) !== `payments.fields.${k}` ? t(`payments.fields.${k}`) : k} value={settings[k]} onChange={(v) => setSettings({ ...settings, [k]: v })} />
            )
          )}
          {gateway.id === "bacs" && (
            <fieldset className="fieldset stack">
              <legend>{t("payments.accounts")}</legend>
              <div className="table-wrap">
                <table className="table">
                  <thead>
                    <tr>
                      {ACCOUNT_FIELDS.map((f) => (
                        <th key={f} scope="col">
                          {t(`payments.${f}`)}
                        </th>
                      ))}
                      <th scope="col">
                        <span className="sr-only">{t("common.actions")}</span>
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {accounts.map((a, i) => (
                      <tr key={i}>
                        {ACCOUNT_FIELDS.map((f) => (
                          <td key={f}>
                            <input
                              className="input input-sm"
                              aria-label={t(`payments.${f}`)}
                              value={a[f] || ""}
                              onChange={(e) => setAccounts(accounts.map((x, j) => (j === i ? { ...x, [f]: e.target.value } : x)))}
                            />
                          </td>
                        ))}
                        <td>
                          <button type="button" className="icon-btn danger" onClick={() => setAccounts(accounts.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                            <FiTrash2 />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div>
                <button type="button" className="btn btn-secondary btn-sm" onClick={() => setAccounts([...accounts, {}])}>
                  <FiPlus aria-hidden="true" /> {t("payments.addAccount")}
                </button>
              </div>
            </fieldset>
          )}
          <div>
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {t("common.saveChanges")}
            </button>
          </div>
        </form>
      )}
    </section>
  );
};

const PaymentsTab = () => {
  const { t } = useI18n();
  const { reload: reloadShop } = useShop();
  const { data, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/payment-gateways")).data), [], []);
  if (loading) return <Spinner />;
  const offline = data.filter((g) => g.type !== "online" && !ONLINE_PROVIDERS.includes(g.id));
  return (
    <div className="stack-lg">
      <h2>{t("payments.offlineTitle")}</h2>
      <ErrorBox error={error} />
      {[...offline]
        .sort((a, b) => (a.order || 0) - (b.order || 0))
        .map((g) => (
          <Gateway
            key={g.id}
            gateway={g}
            onSaved={() => {
              reload();
              reloadShop();
            }}
          />
        ))}
      <OnlinePayments onSaved={reloadShop} />
    </div>
  );
};

export default PaymentsTab;
