import { useCallback, useEffect, useState } from "react";
import { toast } from "react-toastify";
import { QRCodeSVG } from "qrcode.react";
import { FiTrash2 } from "react-icons/fi";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { attestationToJSON, preparePublicKeyCreationOptions, webauthnSupported } from "../../lib/webauthn";
import { ConfirmModal, ErrorBox, Modal, Spinner, TextField, Toggle } from "../../components/ui";

// Security is the self-service MFA page (ported from cwclock's MfaSettings):
// TOTP enrolment with QR + confirm + disable, and WebAuthn security keys.
const Security = () => {
  const { t, errMsg, formatDate } = useI18n();
  const { refreshMe } = useAuth();
  const [status, setStatus] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [setup, setSetup] = useState(null);
  const [code, setCode] = useState("");
  const [disabling, setDisabling] = useState(false);
  const [deleting, setDeleting] = useState(null);
  const [pending, setPending] = useState(null);
  const [keyName, setKeyName] = useState("");

  const refresh = useCallback(async () => {
    try {
      const { data } = await api.get("/v1/users/me/mfa");
      setStatus(data);
    } catch (err) {
      setError(errMsg(err));
    }
  }, [errMsg]);

  useEffect(() => {
    refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const run = async (fn) => {
    setError(null);
    setBusy(true);
    try {
      await fn();
    } catch (err) {
      setError(err?.response ? errMsg(err) : t("auth.securityKeyFailed"));
    } finally {
      setBusy(false);
    }
  };

  const startTotp = () =>
    run(async () => {
      const { data } = await api.post("/v1/users/me/mfa/totp/setup", {});
      setSetup(data);
    });

  const confirmTotp = (e) => {
    e.preventDefault();
    run(async () => {
      await api.post("/v1/users/me/mfa/totp/confirm", { code: code.trim() });
      setSetup(null);
      setCode("");
      toast.success(t("security.totpEnabled"));
      await refresh();
      refreshMe().catch(() => {});
    });
  };

  const disableTotp = () =>
    run(async () => {
      setDisabling(false);
      await api.delete("/v1/users/me/mfa/totp");
      await refresh();
      refreshMe().catch(() => {});
    });

  const registerKey = () =>
    run(async () => {
      const { data: begin } = await api.post("/v1/users/me/mfa/webauthn/register/begin", {});
      const credential = await navigator.credentials.create(preparePublicKeyCreationOptions(begin.options));
      setPending({ ceremonyToken: begin.ceremonyToken, credential: attestationToJSON(credential) });
      setKeyName("");
    });

  const finishKey = (e) => {
    e?.preventDefault?.();
    run(async () => {
      await api.post("/v1/users/me/mfa/webauthn/register/finish", { ...pending, name: keyName.trim() || t("security.defaultKeyName") });
      setPending(null);
      toast.success(t("security.keyAdded"));
      await refresh();
      refreshMe().catch(() => {});
    });
  };

  const deleteKey = () =>
    run(async () => {
      const cred = deleting;
      setDeleting(null);
      await api.delete(`/v1/users/me/mfa/webauthn/${encodeURIComponent(cred.id)}`);
      await refresh();
      refreshMe().catch(() => {});
    });

  if (!status) return error ? <ErrorBox error={error} /> : <Spinner />;
  const keys = status.webauthn || status.webauthnCredentials || [];
  const qr = setup?.qrCode || setup?.qrCodePng;

  return (
    <div className="stack-lg">
      <section className="card stack">
        <h2>{t("security.totpTitle")}</h2>
        <p className="muted">{t("security.totpIntro")}</p>
        <Toggle
          checked={!!status.totpEnabled}
          disabled={busy || (!status.totpEnabled && !!setup)}
          onChange={() => (status.totpEnabled ? setDisabling(true) : startTotp())}
          label={status.totpEnabled ? t("security.totpOn") : t("security.totpOff")}
        />
        {setup && !status.totpEnabled && (
          <form className="totp-setup stack" onSubmit={confirmTotp}>
            <p>{t("security.scan")}</p>
            {qr ? <img src={qr} alt={t("security.qrAlt")} className="qr" width="200" height="200" /> : setup.otpauthUrl && <QRCodeSVG value={setup.otpauthUrl} size={200} title={t("security.qrAlt")} />}
            <p className="small">
              {t("security.secret")} <code>{setup.secret}</code>
            </p>
            <TextField label={t("auth.mfaCode")} value={code} onChange={setCode} inputMode="numeric" autoComplete="one-time-code" required />
            <div className="row">
              <button type="submit" className="btn btn-primary" disabled={busy || !code.trim()}>
                {t("security.confirm")}
              </button>
              <button type="button" className="btn btn-secondary" onClick={() => setSetup(null)}>
                {t("common.cancel")}
              </button>
            </div>
          </form>
        )}
      </section>

      <section className="card stack">
        <div className="section-head">
          <h2>{t("security.keysTitle")}</h2>
          {webauthnSupported() && (
            <button type="button" className="btn btn-secondary btn-sm" onClick={registerKey} disabled={busy}>
              {t("security.addKey")}
            </button>
          )}
        </div>
        <p className="muted">{t("security.keysIntro")}</p>
        {!webauthnSupported() && <p className="notice notice-info">{t("security.noWebauthn")}</p>}
        {keys.length === 0 ? (
          <p className="muted small">{t("security.noKeys")}</p>
        ) : (
          <ul className="plain-list">
            {keys.map((k) => (
              <li key={k.id} className="list-row">
                <span>
                  <strong>{k.name}</strong> <span className="muted small">{formatDate(k.createdAt)}</span>
                </span>
                <button type="button" className="icon-btn danger" onClick={() => setDeleting(k)} aria-label={t("security.deleteKeyNamed", { name: k.name })}>
                  <FiTrash2 />
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <ErrorBox error={error} />

      <ConfirmModal show={disabling} title={t("security.disableTitle")} body={t("security.disableBody")} confirmLabel={t("security.disable")} onConfirm={disableTotp} onCancel={() => setDisabling(false)} />
      <ConfirmModal
        show={!!deleting}
        title={t("security.deleteKeyTitle")}
        body={deleting ? t("security.deleteKeyBody", { name: deleting.name }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={deleteKey}
        onCancel={() => setDeleting(null)}
      />
      <Modal
        show={!!pending}
        title={t("security.nameKey")}
        onClose={() => setPending(null)}
        footer={
          <button type="button" className="btn btn-primary" onClick={finishKey} disabled={busy}>
            {t("common.save")}
          </button>
        }
      >
        <form onSubmit={finishKey}>
          <TextField label={t("security.keyName")} value={keyName} onChange={setKeyName} placeholder={t("security.defaultKeyName")} autoFocus />
        </form>
      </Modal>
    </div>
  );
};

export default Security;
