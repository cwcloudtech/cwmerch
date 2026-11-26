import { useState } from "react";
import { toast } from "react-toastify";
import { FiCopy, FiExternalLink, FiTrash2 } from "react-icons/fi";
import { api, apiUrl, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useI18n } from "../../i18n/I18nContext";
import { ConfirmModal, Empty, ErrorBox, Modal, Spinner, TextField } from "../../components/ui";

// ApiKeys: long-term API keys (X-Api-Key). Ported from cwclock: the plaintext
// token is shown once, right after creation.
const ApiKeys = () => {
  const { t, errMsg, formatDate } = useI18n();
  const { data: keys, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/users/me/api-keys")).data), [], []);
  const [form, setForm] = useState({ description: "", expiresAt: "" });
  const [created, setCreated] = useState(null);
  const [deleting, setDeleting] = useState(null);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState(null);

  const create = async (e) => {
    e.preventDefault();
    setBusy(true);
    setFormError(null);
    try {
      const { data } = await api.post("/v1/users/me/api-keys", {
        description: form.description.trim(),
        expiresAt: form.expiresAt ? `${form.expiresAt}T23:59:59Z` : null,
      });
      setCreated(data.token);
      setForm({ description: "", expiresAt: "" });
      reload();
    } catch (err) {
      setFormError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    const k = deleting;
    setDeleting(null);
    try {
      await api.delete(`/v1/users/me/api-keys/${k.id}`);
      toast.success(t("apiKeys.deleted"));
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const copy = (text) =>
    navigator.clipboard?.writeText(text).then(
      () => toast.success(t("common.copied")),
      () => {}
    );

  return (
    <div className="stack-lg">
      <div className="section-head">
        <p className="muted">{t("apiKeys.intro")}</p>
        <a className="btn btn-secondary btn-sm" href={apiUrl()} target="_blank" rel="noopener noreferrer">
          <FiExternalLink aria-hidden="true" /> {t("apiKeys.openSwagger")}
        </a>
      </div>
      <form className="card stack" onSubmit={create}>
        <h2>{t("apiKeys.create")}</h2>
        <div className="form-grid">
          <TextField label={t("apiKeys.description")} value={form.description} onChange={(description) => setForm({ ...form, description })} required />
          <TextField label={t("apiKeys.expiresAt")} hint={t("apiKeys.expiresHint")} type="date" value={form.expiresAt} onChange={(expiresAt) => setForm({ ...form, expiresAt })} />
        </div>
        <ErrorBox error={formError} />
        <div>
          <button type="submit" className="btn btn-primary" disabled={busy || !form.description.trim()}>
            {t("common.create")}
          </button>
        </div>
      </form>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : keys.length === 0 ? (
        <Empty title={t("apiKeys.empty")} />
      ) : (
        <ul className="plain-list">
          {keys.map((k) => (
            <li key={k.id} className="list-row">
              <span>
                <strong>{k.description}</strong>{" "}
                <span className="muted small">{k.expiresAt ? t("apiKeys.expiresOn", { date: formatDate(k.expiresAt) }) : t("apiKeys.never")}</span>
              </span>
              <button type="button" className="icon-btn danger" onClick={() => setDeleting(k)} aria-label={t("apiKeys.deleteNamed", { name: k.description })}>
                <FiTrash2 />
              </button>
            </li>
          ))}
        </ul>
      )}
      <Modal show={!!created} title={t("apiKeys.createdTitle")} onClose={() => setCreated(null)}>
        <p className="notice notice-info">{t("apiKeys.createdWarning")}</p>
        <div className="token-box">
          <code>{created}</code>
          <button type="button" className="icon-btn" onClick={() => copy(created)} aria-label={t("common.copy")}>
            <FiCopy />
          </button>
        </div>
      </Modal>
      <ConfirmModal
        show={!!deleting}
        title={t("apiKeys.deleteTitle")}
        body={deleting ? t("apiKeys.deleteBody", { name: deleting.description }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={remove}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default ApiKeys;
