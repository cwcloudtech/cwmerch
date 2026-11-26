import { useState } from "react";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { assertionToJSON, preparePublicKeyRequestOptions, webauthnSupported } from "../../lib/webauthn";
import { ErrorBox } from "../../components/ui";

// MfaChallenge is the second login step once /v1/users/login answered
// {mfaRequired, challengeToken, hasTotp, hasWebAuthn}: an authenticator code
// and/or a security key (WebAuthn / YubiKey). Ported from cwclock.
const MfaChallenge = ({ challenge, onDone, onCancel }) => {
  const { t, errMsg } = useI18n();
  const { setSession } = useAuth();
  const [code, setCode] = useState("");
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  const submitTotp = async (e) => {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      const { data } = await api.post("/v1/users/login/mfa/totp", { challengeToken: challenge.challengeToken, code: code.trim() });
      setSession(data);
      onDone?.(data);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const useKey = async () => {
    setError(null);
    setBusy(true);
    try {
      const { data: begin } = await api.post("/v1/users/login/mfa/webauthn/begin", { challengeToken: challenge.challengeToken });
      const assertion = await navigator.credentials.get(preparePublicKeyRequestOptions(begin.options));
      const { data } = await api.post("/v1/users/login/mfa/webauthn/finish", { ceremonyToken: begin.ceremonyToken, credential: assertionToJSON(assertion) });
      setSession(data);
      onDone?.(data);
    } catch (err) {
      setError(err?.response ? errMsg(err) : t("auth.securityKeyFailed"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="card stack auth-card">
      <h2>{t("auth.mfaTitle")}</h2>
      <p className="muted">{t("auth.mfaIntro")}</p>
      {challenge.hasTotp && (
        <form className="stack" onSubmit={submitTotp}>
          <label className="field-label" htmlFor="mfa-code">
            {t("auth.mfaCode")}
          </label>
          <input
            id="mfa-code"
            className="input"
            type="text"
            inputMode="numeric"
            autoComplete="one-time-code"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            autoFocus
          />
          <button type="submit" className="btn btn-primary" disabled={busy || !code.trim()}>
            {t("auth.mfaVerify")}
          </button>
        </form>
      )}
      {challenge.hasWebAuthn && webauthnSupported() && (
        <button type="button" className="btn btn-secondary" disabled={busy} onClick={useKey}>
          {t("auth.mfaUseKey")}
        </button>
      )}
      <ErrorBox error={error} />
      <button type="button" className="link-btn" onClick={onCancel}>
        {t("auth.backToLogin")}
      </button>
    </div>
  );
};

export default MfaChallenge;
