import { useState } from "react";
import { toast } from "react-toastify";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, SelectField, Spinner, TextArea, TextField, Toggle } from "../../../components/ui";

const EmailRow = ({ email, onSaved }) => {
  const { t, errMsg } = useI18n();
  const [open, setOpen] = useState(false);
  const [e, setE] = useState(email);
  const [busy, setBusy] = useState(false);
  const set = (k) => (v) => setE((x) => ({ ...x, [k]: v }));
  const save = async (next) => {
    setBusy(true);
    try {
      await api.put(`/v1/admin/emails/${email.id}`, { ...e, ...next });
      toast.success(t("admin.saved"));
      onSaved();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <li className="card stack">
      <div className="section-head">
        <div>
          <strong>{email.title}</strong> <span className="badge">{email.customerEmail ? t("emails.customer") : email.recipient || t("emails.admin")}</span>
          <p className="muted small">{email.description}</p>
        </div>
        <div className="row">
          <Toggle
            checked={!!e.enabled}
            disabled={busy}
            onChange={(enabled) => {
              setE((x) => ({ ...x, enabled }));
              save({ enabled });
            }}
            label={<span className="sr-only">{t("emails.enable", { name: email.title })}</span>}
          />
          <button type="button" className="btn btn-secondary btn-sm" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
            {open ? t("common.close") : t("payments.manage")}
          </button>
        </div>
      </div>
      {open && (
        <form
          className="stack"
          onSubmit={(ev) => {
            ev.preventDefault();
            save();
          }}
        >
          {!email.customerEmail && <TextField label={t("emails.recipient")} hint={t("emails.recipientHint")} value={e.recipient} onChange={set("recipient")} />}
          <TextField label={t("emails.subject")} hint={t("emails.placeholdersHint")} value={e.subject} onChange={set("subject")} />
          <TextField label={t("emails.heading")} value={e.heading} onChange={set("heading")} />
          <TextArea label={t("emails.additional")} value={e.additionalContent} onChange={set("additionalContent")} rows={3} />
          <SelectField label={t("emails.type")} value={e.emailType || "html"} onChange={set("emailType")} options={["html", "plain", "multipart"].map((x) => ({ value: x, label: t(`emails.types.${x}`) }))} />
          <div>
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {t("common.saveChanges")}
            </button>
          </div>
        </form>
      )}
    </li>
  );
};

const EmailsTab = () => {
  const { t } = useI18n();
  const { data, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/emails")).data), [], []);
  if (loading) return <Spinner />;
  return (
    <div className="stack">
      <p className="muted">{t("emails.intro")}</p>
      <ErrorBox error={error} />
      <ul className="plain-list stack">
        {data.map((e) => (
          <EmailRow key={e.id} email={e} onSaved={reload} />
        ))}
      </ul>
    </div>
  );
};

export default EmailsTab;
