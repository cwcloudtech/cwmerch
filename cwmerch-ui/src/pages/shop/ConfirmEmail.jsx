import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import AuthLogo from "../../components/AuthLogo";
import { ErrorBox } from "../../components/ui";

// ConfirmEmail applies an email change from the link sent to the new address.
// It asks for a click (rather than confirming on load) so a mail scanner
// opening the link can't confirm it; it works signed out or on another device.
const ConfirmEmail = () => {
  const { t, errMsg } = useI18n();
  const { user, refreshMe } = useAuth();
  const [params] = useSearchParams();
  const token = params.get("token") || "";
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [done, setDone] = useState("");
  useSeo({ title: t("account.emailChange.confirmTitle"), noindex: true });

  const confirm = async () => {
    setBusy(true);
    setError(null);
    try {
      const { data } = await api.post("/v1/users/email/confirm", { token });
      setDone(data?.email || "");
      if (user) await refreshMe().catch(() => {});
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container narrow">
      <AuthLogo />
      <h1>{t("account.emailChange.confirmTitle")}</h1>
      {!token ? (
        <p className="notice notice-error">{t("errors.invalidToken")}</p>
      ) : done ? (
        <div className="card stack">
          <p className="notice notice-success">{t("account.emailChange.confirmed", { email: done })}</p>
          <Link className="btn btn-primary" to={user ? "/my-account/edit-account" : "/my-account"}>
            {user ? t("nav.myAccount") : t("nav.signIn")}
          </Link>
        </div>
      ) : (
        <div className="card stack">
          <p>{t("account.emailChange.confirmIntro")}</p>
          <ErrorBox error={error} />
          <button type="button" className="btn btn-primary" onClick={confirm} disabled={busy}>
            {t("account.emailChange.confirmButton")}
          </button>
        </div>
      )}
    </div>
  );
};

export default ConfirmEmail;
