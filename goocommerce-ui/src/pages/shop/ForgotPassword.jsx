import { useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { ErrorBox, TextField } from "../../components/ui";

const ForgotPassword = () => {
  const { t, errMsg } = useI18n();
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState(null);
  useSeo({ title: t("auth.forgotTitle"), noindex: true });

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post("/v1/users/forgot-password", { email });
      setSent(true);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container narrow">
      <h1>{t("auth.forgotTitle")}</h1>
      {sent ? (
        <p className="notice notice-success">{t("auth.forgotSent")}</p>
      ) : (
        <form className="card stack" onSubmit={submit}>
          <p className="muted">{t("auth.forgotIntro")}</p>
          <TextField label={t("auth.email")} type="email" value={email} onChange={setEmail} required autoComplete="email" autoFocus />
          <ErrorBox error={error} />
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("auth.resetPassword")}
          </button>
        </form>
      )}
      <p>
        <Link to="/my-account">{t("auth.backToLogin")}</Link>
      </p>
    </div>
  );
};

export default ForgotPassword;
