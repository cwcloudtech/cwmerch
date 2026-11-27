import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../../api/client";
import { useShop } from "../../../context/ShopContext";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Spinner, TextField } from "../../../components/ui";

// CWCloudTab: the CWCloud email API key and the contact form
// (GET/PUT /v1/admin/cwcloud-settings). Everything lives in WordPress' own
// tables: the key in the CWCloud plugin's cwcloud-api-secret option, the
// contact recipient in Contact Form 7's form (or the admin email).
const toForm = (d) => ({
  apiUrl: d.apiUrl || "",
  apiKey: "",
  contactFormId: d.contactFormId || "",
  contactRecipient: d.contactRecipient || "",
  contactSubject: d.contactSubject || "",
  emailFrom: d.emailFrom || "",
});

const CWCloudTab = () => {
  const { t, errMsg } = useI18n();
  const { reload: reloadShop } = useShop();
  const [data, setData] = useState(null);
  const [form, setForm] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [testTo, setTestTo] = useState("");

  useEffect(() => {
    api
      .get("/v1/admin/cwcloud-settings")
      .then(({ data: d }) => {
        setData(d);
        setForm(toForm(d));
      })
      .catch(setError);
  }, []);

  if (!form) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const body = { ...form };
      if (!body.apiKey) delete body.apiKey; // blank keeps the stored key
      const { data: d } = await api.put("/v1/admin/cwcloud-settings", body);
      setData(d);
      setForm(toForm(d));
      toast.success(t("admin.saved"));
      reloadShop();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const clearKey = async () => {
    setBusy(true);
    try {
      const { data: d } = await api.put("/v1/admin/cwcloud-settings", { clearApiKey: true });
      setData(d);
      toast.success(t("admin.saved"));
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const test = async () => {
    try {
      const { data: d } = await api.post("/v1/admin/cwcloud-settings/test-email", { to: testTo });
      if (d?.sent) toast.success(t("cwcloud.testSent", { to: testTo }));
      else toast.error(t("cwcloud.testFailed"));
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <form className="stack-lg" onSubmit={save}>
      <section className="card stack settings-section">
        <h2>{t("cwcloud.title")}</h2>
        <p className="muted small">{t("cwcloud.intro")}</p>
        <TextField label={t("cwcloud.apiUrl")} dir="ltr" type="url" value={form.apiUrl} onChange={set("apiUrl")} placeholder="https://api.cwcloud.tech" />
        <TextField
          label={t("cwcloud.apiKey")}
          dir="ltr"
          type="password"
          autoComplete="new-password"
          value={form.apiKey}
          onChange={set("apiKey")}
          placeholder={data.apiKeySet ? t("cwcloud.apiKeyStored") : ""}
          hint={data.apiKeySet ? t(`cwcloud.apiKeySource.${data.apiKeySource || "wordpress"}`) : t("cwcloud.apiKeyMissing")}
        />
        {data.apiKeySet && data.apiKeySource === "wordpress" && (
          <div>
            <button type="button" className="btn btn-secondary btn-sm" onClick={clearKey} disabled={busy}>
              {t("cwcloud.clearKey")}
            </button>
          </div>
        )}
        <TextField label={t("cwcloud.emailFrom")} dir="ltr" type="email" value={form.emailFrom} onChange={set("emailFrom")} hint={t("cwcloud.emailFromHint")} />
        <div className="inline-form">
          <TextField label={t("cwcloud.testTo")} dir="ltr" type="email" value={testTo} onChange={setTestTo} />
          <button type="button" className="btn btn-secondary" onClick={test} disabled={!testTo.includes("@")}>
            {t("cwcloud.sendTest")}
          </button>
        </div>
      </section>
      <section className="card stack settings-section">
        <h2>{t("cwcloud.contactTitle")}</h2>
        <p className="muted small">{t(`cwcloud.contactMode.${data.contactMode}`)}</p>
        <TextField label={t("cwcloud.contactFormId")} dir="ltr" value={form.contactFormId} onChange={set("contactFormId")} hint={t("cwcloud.contactFormIdHint")} />
        <TextField
          label={t("cwcloud.contactRecipient")}
          dir="ltr"
          type="email"
          value={form.contactRecipient}
          onChange={set("contactRecipient")}
          hint={t(`cwcloud.recipientSource.${data.recipientSource || "admin_email"}`, { form: data.contactForm7 || "" })}
        />
        <TextField label={t("cwcloud.contactSubject")} value={form.contactSubject} onChange={set("contactSubject")} />
      </section>
      <div className="sticky-save">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

export default CWCloudTab;
