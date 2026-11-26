import { useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { ErrorBox, TextField } from "../../components/ui";

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
      <TextField label={t("auth.email")} value={user.email} onChange={() => {}} disabled />
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
