import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { FiArrowDown, FiArrowUp, FiEdit2, FiShare2, FiStar, FiTrash2 } from "react-icons/fi";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { searchShareable } from "./useAi";
import { ConfirmModal, ErrorBox, Field, Modal, TextField } from "../../components/ui";

const emptyForm = { provider: "", name: "", token: "", model: "", baseUrl: "", maxTokens: "" };

const randomSuffix = () => {
  const bytes = new Uint8Array(4);
  window.crypto?.getRandomValues?.(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
};

const memberLabel = (s) => [s.name, s.surname].filter(Boolean).join(" ") || s.email;

// Subscriptions manages AI connections (ported from cwclock's AiSubscriptions):
// provider catalogue, CRUD, default (= first) + reorder, sharing by email.
const Subscriptions = ({ ai }) => {
  const { t, errMsg } = useI18n();
  const { user } = useAuth();
  const { providers, connections } = ai;
  const [showForm, setShowForm] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form, setForm] = useState(emptyForm);
  const [suffix, setSuffix] = useState(randomSuffix);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(null);
  const [sharing, setSharing] = useState(null);
  const [shares, setShares] = useState([]);
  const [shareEmail, setShareEmail] = useState("");
  const [suggestions, setSuggestions] = useState([]);

  useEffect(() => {
    if (!sharing) return undefined;
    const term = shareEmail.trim();
    if (term.length < 2) {
      setSuggestions([]);
      return undefined;
    }
    let cancelled = false;
    const handle = setTimeout(async () => {
      const r = await searchShareable(term);
      if (!cancelled) setSuggestions(r);
    }, 300);
    return () => {
      cancelled = true;
      clearTimeout(handle);
    };
  }, [shareEmail, sharing]);

  const selected = providers.find((p) => p.name === form.provider);
  const effectiveModel = form.model.trim() || selected?.defaultModel || "";
  const suggestedName = effectiveModel ? `${effectiveModel}-${suffix}` : "";
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));
  const owned = (c) => c.ownerId === user?.id || c.role === "owner";

  const openAdd = () => {
    setEditing(null);
    setForm(emptyForm);
    setSuffix(randomSuffix());
    setError(null);
    setShowForm(true);
  };
  const openEdit = (c) => {
    setEditing(c);
    setForm({ provider: c.provider || "", name: c.name || "", token: "", model: c.model || "", baseUrl: c.baseUrl || "", maxTokens: c.maxTokens ? String(c.maxTokens) : "" });
    setError(null);
    setShowForm(true);
  };

  const submit = async (e) => {
    e.preventDefault();
    setError(null);
    if (!form.provider) return setError(t("ai.pickProviderError"));
    if (!editing && !form.token.trim()) return setError(t("ai.keyRequired"));
    const payload = {
      provider: form.provider,
      name: form.name.trim() || suggestedName,
      token: form.token.trim(),
      model: form.model.trim(),
      baseUrl: form.baseUrl.trim(),
      maxTokens: form.maxTokens ? Number(form.maxTokens) : 0,
    };
    setBusy(true);
    try {
      if (editing) await ai.updateConnection(editing.id, payload);
      else await ai.createConnection(payload);
      setShowForm(false);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const tryIt = async (fn) => {
    try {
      await fn();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const move = (index, delta) => {
    const ids = connections.map((c) => c.id);
    const j = index + delta;
    if (j < 0 || j >= ids.length) return;
    [ids[index], ids[j]] = [ids[j], ids[index]];
    tryIt(() => ai.reorderConnections(ids));
  };
  const makeDefault = (c) => tryIt(() => ai.reorderConnections([c.id, ...connections.filter((x) => x.id !== c.id).map((x) => x.id)]));

  const openShares = async (c) => {
    setSharing(c);
    setShareEmail("");
    try {
      setShares(await ai.listShares(c.id));
    } catch {
      setShares([]);
    }
  };
  const addShare = (e) => {
    e.preventDefault();
    if (!shareEmail.trim()) return;
    tryIt(async () => {
      const updated = await ai.share(sharing.id, shareEmail.trim());
      setShares(updated.length ? updated : await ai.listShares(sharing.id));
      setShareEmail("");
    });
  };
  const removeShare = (userId) =>
    tryIt(async () => {
      await ai.unshare(sharing.id, userId);
      setShares((prev) => prev.filter((s) => s.userId !== userId));
    });

  return (
    <div className="stack">
      <div className="section-head">
        <h2>{t("ai.subscriptions")}</h2>
        <button type="button" className="btn btn-primary btn-sm" onClick={openAdd}>
          {t("ai.addSubscription")}
        </button>
      </div>
      <p className="muted">{t("ai.intro")}</p>
      {connections.length === 0 ? (
        <p className="muted">{t("ai.noSubscriptions")}</p>
      ) : (
        <ul className="plain-list">
          {connections.map((c, i) => (
            <li key={c.id} className="list-row">
              <span>
                <strong>{c.name || c.provider}</strong> {i === 0 && <span className="badge badge-processing">{t("ai.default")}</span>}{" "}
                {!owned(c) && <span className="badge">{t("ai.shared")}</span>}
                <br />
                <span className="muted small">
                  {c.provider}
                  {c.model ? ` · ${c.model}` : ""}
                </span>
              </span>
              <span className="row-actions">
                <button type="button" className="icon-btn" onClick={() => move(i, -1)} disabled={i === 0} aria-label={t("common.moveUp")}>
                  <FiArrowUp />
                </button>
                <button type="button" className="icon-btn" onClick={() => move(i, 1)} disabled={i === connections.length - 1} aria-label={t("common.moveDown")}>
                  <FiArrowDown />
                </button>
                {i !== 0 && (
                  <button type="button" className="icon-btn" onClick={() => makeDefault(c)} aria-label={t("ai.setDefault")} title={t("ai.setDefault")}>
                    <FiStar />
                  </button>
                )}
                {owned(c) && (
                  <>
                    <button type="button" className="icon-btn" onClick={() => openShares(c)} aria-label={t("ai.share")} title={t("ai.share")}>
                      <FiShare2 />
                    </button>
                    <button type="button" className="icon-btn" onClick={() => openEdit(c)} aria-label={t("common.edit")} title={t("common.edit")}>
                      <FiEdit2 />
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => setDeleting(c)} aria-label={t("common.delete")} title={t("common.delete")}>
                      <FiTrash2 />
                    </button>
                  </>
                )}
              </span>
            </li>
          ))}
        </ul>
      )}

      <Modal show={showForm} title={editing ? t("ai.editSubscription") : t("ai.addSubscription")} onClose={() => setShowForm(false)}>
        <form className="stack" onSubmit={submit}>
          <Field label={t("ai.provider")} required>
            {(id) => (
              <select id={id} className="input" value={form.provider} onChange={(e) => set("provider")(e.target.value)} required>
                <option value="">{t("ai.pickProvider")}</option>
                {providers.map((p) => (
                  <option key={p.name} value={p.name}>
                    {p.label || p.name}
                  </option>
                ))}
              </select>
            )}
          </Field>
          <TextField label={t("ai.name")} value={form.name} onChange={set("name")} placeholder={suggestedName || t("ai.namePlaceholder")} />
          <TextField
            label={t("ai.apiKey")}
            type="password"
            autoComplete="off"
            value={form.token}
            onChange={set("token")}
            placeholder={editing ? t("ai.keyLeaveBlank") : selected?.keyHint || ""}
            required={!editing}
          />
          <Field label={t("ai.model")}>
            {(id) => (
              <>
                <input id={id} className="input" list="ai-models" value={form.model} onChange={(e) => set("model")(e.target.value)} placeholder={selected?.defaultModel || ""} />
                <datalist id="ai-models">
                  {(selected?.models || []).map((m) => (
                    <option key={m} value={m} />
                  ))}
                </datalist>
              </>
            )}
          </Field>
          <TextField label={t("ai.baseUrl")} value={form.baseUrl} onChange={set("baseUrl")} placeholder={selected?.defaultUrl || "https://..."} />
          <TextField label={t("ai.maxTokens")} type="number" min="1" value={form.maxTokens} onChange={set("maxTokens")} />
          <ErrorBox error={error} />
          <div className="row end">
            <button type="button" className="btn btn-secondary" onClick={() => setShowForm(false)}>
              {t("common.cancel")}
            </button>
            <button type="submit" className="btn btn-primary" disabled={busy}>
              {editing ? t("common.save") : t("ai.addSubscription")}
            </button>
          </div>
        </form>
      </Modal>

      <Modal show={!!sharing} title={t("ai.shareTitle")} onClose={() => setSharing(null)}>
        <p className="muted">{t("ai.shareIntro")}</p>
        <form className="row" onSubmit={addShare}>
          <label htmlFor="share-email" className="sr-only">
            {t("auth.email")}
          </label>
          <input id="share-email" className="input" type="email" list="share-suggestions" value={shareEmail} onChange={(e) => setShareEmail(e.target.value)} placeholder={t("ai.shareEmailPlaceholder")} />
          <datalist id="share-suggestions">
            {suggestions.map((s) => (
              <option key={s.email} value={s.email} label={memberLabel(s)} />
            ))}
          </datalist>
          <button type="submit" className="btn btn-primary btn-sm" disabled={!shareEmail.trim()}>
            {t("ai.shareAdd")}
          </button>
        </form>
        {shares.length === 0 ? (
          <p className="muted small">{t("ai.shareEmpty")}</p>
        ) : (
          <ul className="plain-list">
            {shares.map((s) => (
              <li key={s.userId} className="list-row">
                <span>{memberLabel(s)}</span>
                <button type="button" className="icon-btn danger" onClick={() => removeShare(s.userId)} aria-label={t("ai.shareRemove")}>
                  <FiTrash2 />
                </button>
              </li>
            ))}
          </ul>
        )}
      </Modal>

      <ConfirmModal
        show={!!deleting}
        title={t("ai.deleteTitle")}
        body={deleting ? t("ai.deleteBody", { name: deleting.name || deleting.provider }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={() => {
          const c = deleting;
          setDeleting(null);
          tryIt(() => ai.deleteConnection(c.id));
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default Subscriptions;
