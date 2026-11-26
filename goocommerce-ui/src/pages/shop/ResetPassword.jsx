import { useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { ErrorBox, TextField } from "../../components/ui";

const ResetPassword = () => {
  const { t, errMsg } = useI18n();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const token = params.get("token") || "";
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  useSeo({ title: t("auth.resetTitle"), noindex: true });

  const submit = async (e) => {
    e.preventDefault();
    if (password !== confirmPassword) {
      setError(t("errors.passwordsMismatch"));
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.post("/v1/users/reset-password", { token, password, confirmPassword });
      toast.success(t("auth.resetDone"));
      navigate("/my-account", { replace: true });
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container narrow">
      <h1>{t("auth.resetTitle")}</h1>
      {!token ? (
        <p className="notice notice-error">
          {t("auth.resetMissingToken")} <Link to="/forgot-password">{t("auth.forgotTitle")}</Link>
        </p>
      ) : (
        <form className="card stack" onSubmit={submit}>
          <TextField label={t("auth.newPassword")} type="password" value={password} onChange={setPassword} required autoComplete="new-password" autoFocus />
          <TextField label={t("auth.confirmPassword")} type="password" value={confirmPassword} onChange={setConfirmPassword} required autoComplete="new-password" />
          <ErrorBox error={error} />
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("auth.savePassword")}
          </button>
        </form>
      )}
    </div>
  );
};

export default ResetPassword;
