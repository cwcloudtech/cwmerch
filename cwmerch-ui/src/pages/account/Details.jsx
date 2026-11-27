import { useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { ErrorBox, TextField } from "../../components/ui";

// EmailChange: a new email only replaces the current one once confirmed
// through the link sent to it (POST /v1/users/me/email).
const EmailChange = () => {
  const { user } = useAuth();
  const { t, errMsg } = useI18n();
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [pending, setPending] = useState("");

  const send = async () => {
    setBusy(true);
    setError(null);
    try {
      const { data } = await api.post("/v1/users/me/email", { email: email.trim() });
      setPending(data?.pendingEmail || email.trim());
      setOpen(false);
      setEmail("");
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <fieldset className="fieldset stack">
      <legend>{t("auth.email")}</legend>
      <div className="row wrap">
        <span className="grow">
          <bdi dir="ltr">{user.email}</bdi>
        </span>
        {!open && (
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setOpen(true)}>
            {t("account.emailChange.change")}
          </button>
        )}
      </div>
      {pending && <p className="notice notice-success">{t("account.emailChange.sent", { email: pending })}</p>}
      {open && (
        <div className="stack">
          <TextField label={t("account.emailChange.newEmail")} hint={t("account.emailChange.hint")} type="email" value={email} onChange={setEmail} autoComplete="email" autoFocus
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                if (email.trim() && !busy) send();
              }
            }}
          />
          <ErrorBox error={error} />
          <div className="row">
            <button type="button" className="btn btn-primary btn-sm" onClick={send} disabled={busy || !email.trim()}>
              {t("account.emailChange.send")}
            </button>
            <button type="button" className="btn btn-secondary btn-sm" onClick={() => (setOpen(false), setError(null))}>
              {t("common.cancel")}
            </button>
          </div>
        </div>
      )}
    </fieldset>
  );
};

const Details = () => {
  const { user, refreshMe } = useAuth();
  const { t, errMsg } = useI18n();
  const [form, setForm] = useState({ name: user.name || "", surname: user.surname || "", displayName: user.displayName || "", password: "", confirmPassword: "" });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const set = (k) => (v) => setForm({ ...form, [k]: v });

  const submit = async (e) => {
    e.preventDefault();
    setError(null);
    if (form.password && form.password !== form.confirmPassword) {
      setError(t("errors.passwordsMismatch"));
      return;
    }
    setBusy(true);
    try {
      const body = { name: form.name, surname: form.surname, displayName: form.displayName };
      if (form.password) {
        body.password = form.password;
        body.confirmPassword = form.confirmPassword;
      }
      await api.put("/v1/users/me", body);
      await refreshMe().catch(() => {});
      setForm((f) => ({ ...f, password: "", confirmPassword: "" }));
      toast.success(t("account.detailsSaved"));
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="stack" onSubmit={submit}>
      <div className="form-grid">
        <TextField label={t("auth.firstName")} value={form.name} onChange={set("name")} required autoComplete="given-name" />
        <TextField label={t("auth.lastName")} value={form.surname} onChange={set("surname")} required autoComplete="family-name" />
      </div>
      <TextField label={t("account.displayName")} hint={t("account.displayNameHint")} value={form.displayName} onChange={set("displayName")} />
      <EmailChange />
      <fieldset className="fieldset">
        <legend>{t("account.passwordChange")}</legend>
        <TextField label={t("auth.newPassword")} hint={t("account.passwordBlank")} type="password" value={form.password} onChange={set("password")} autoComplete="new-password" />
        <TextField label={t("auth.confirmPassword")} type="password" value={form.confirmPassword} onChange={set("confirmPassword")} autoComplete="new-password" />
      </fieldset>
      <ErrorBox error={error} />
      <div>
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

export default Details;
