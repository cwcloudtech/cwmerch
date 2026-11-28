import { useState } from "react";
import { toast } from "react-toastify";
import { FiEdit2, FiEye, FiEyeOff, FiPlus, FiTrash2 } from "react-icons/fi";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { stripHtml } from "../../lib/util";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { ConfirmModal, ErrorBox, Modal, Pagination, SelectField, Spinner, TextField, Toggle } from "../../components/ui";
import RichTextEditor from "../../components/RichTextEditor";
import { AnnouncementBanner } from "../../components/AnnouncementBanner";

const STATUSES = ["", "active", "expired", "disabled"];

const statusOf = (a) => (!a.enabled ? "disabled" : a.expired ? "expired" : "active");
const STATUS_BADGE = { active: "completed", expired: "failed", disabled: "pending" };

// datetime-local value (local time) <-> ISO (UTC) for the API.
const pad = (n) => String(n).padStart(2, "0");
const toLocalInput = (iso) => {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
};
const fromLocalInput = (v) => (v ? new Date(v).toISOString() : null);

const EMPTY = { id: 0, title: "", content: "", enabled: true, placement: "home", background: "", dismissible: true, position: 0, expiresAt: null };

const AnnouncementModal = ({ announcement, onClose, onSaved }) => {
  const { t, errMsg } = useI18n();
  const isNew = !announcement.id;
  const [a, setA] = useState({ ...EMPTY, ...announcement });
  const [expires, setExpires] = useState(toLocalInput(announcement.expiresAt));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const set = (k) => (v) => setA((x) => ({ ...x, [k]: v }));
  const customBg = !!a.background;

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body = {
        title: a.title,
        content: a.content,
        enabled: !!a.enabled,
        placement: a.placement,
        background: a.background || "",
        dismissible: !!a.dismissible,
        position: Number(a.position) || 0,
        expiresAt: fromLocalInput(expires),
      };
      if (isNew) await api.post("/v1/admin/announcements", body);
      else await api.put(`/v1/admin/announcements/${a.id}`, body);
      toast.success(t("admin.saved"));
      onSaved();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal show wide title={isNew ? t("admin.announcements.add") : t("admin.announcements.edit")} onClose={onClose}>
      <form className="stack" onSubmit={save}>
        <TextField label={t("admin.announcements.title")} hint={t("admin.announcements.titleHint")} value={a.title} onChange={set("title")} />
        <RichTextEditor label={t("admin.announcements.content")} value={a.content} onChange={set("content")} minHeight={180} />
        <div className="form-grid">
          <SelectField
            label={t("admin.announcements.placement")}
            value={a.placement}
            onChange={set("placement")}
            options={["home", "all"].map((x) => ({ value: x, label: t(`admin.announcements.placements.${x}`) }))}
          />
          <TextField
            label={t("admin.announcements.expiresAt")}
            hint={t("admin.announcements.expiresHint")}
            type="datetime-local"
            value={expires}
            onChange={setExpires}
          />
          <SelectField
            label={t("admin.announcements.background")}
            value={customBg ? "custom" : "theme"}
            onChange={(v) => set("background")(v === "custom" ? a.background || "#0a7fae" : "")}
            options={[
              { value: "theme", label: t("admin.announcements.bgTheme") },
              { value: "custom", label: t("admin.announcements.bgCustom") },
            ]}
          />
          {customBg && <TextField label={t("admin.announcements.color")} type="color" value={a.background} onChange={set("background")} />}
          <TextField label={t("admin.announcements.position")} hint={t("admin.announcements.positionHint")} type="number" value={a.position} onChange={set("position")} />
        </div>
        <div className="row wrap">
          <Toggle checked={!!a.enabled} onChange={set("enabled")} label={t("admin.announcements.enabled")} />
          <Toggle checked={!!a.dismissible} onChange={set("dismissible")} label={t("admin.announcements.dismissible")} />
        </div>
        {stripHtml(a.content) && (
          <fieldset className="fieldset stack">
            <legend>{t("admin.announcements.preview")}</legend>
            <AnnouncementBanner announcement={a} preview />
          </fieldset>
        )}
        <ErrorBox error={error} />
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy || (!stripHtml(a.content) && !/<img/i.test(a.content))}>
            {isNew ? t("admin.announcements.create") : t("common.saveChanges")}
          </button>
        </div>
      </form>
    </Modal>
  );
};

const Announcements = () => {
  const { t, errMsg, formatDateTime } = useI18n();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("");
  const [editing, setEditing] = useState(null);
  const [deleting, setDeleting] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () =>
      normalizePaged(
        (await api.get("/v1/admin/announcements", { params: { page, perPage: 20, search: search || undefined, status: status || undefined } })).data,
        page
      ),
    [page, search, status],
    { items: [], totalPages: 1 }
  );

  const toggle = async (a) => {
    try {
      await api.put(`/v1/admin/announcements/${a.id}`, { ...a, enabled: !a.enabled });
      toast.success(t("admin.saved"));
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.announcements")} subtitle={t("admin.announcements.subtitle")}>
        <button type="button" className="btn btn-primary btn-sm" onClick={() => setEditing(EMPTY)}>
          <FiPlus aria-hidden="true" /> {t("admin.announcements.add")}
        </button>
      </PageHead>
      <div className="toolbar">
        <SelectField
          label={t("admin.announcements.status")}
          value={status}
          onChange={(v) => {
            setStatus(v);
            setPage(1);
          }}
          options={STATUSES.map((s) => ({ value: s, label: t(s ? `admin.announcements.statuses.${s}` : "admin.announcements.allStatuses") }))}
        />
        <SearchBox
          value={search}
          onSearch={(s) => {
            setSearch(s);
            setPage(1);
          }}
        />
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table">
            <thead>
              <tr>
                <th scope="col">{t("admin.announcements.title")}</th>
                <th scope="col">{t("admin.announcements.status")}</th>
                <th scope="col">{t("admin.announcements.placement")}</th>
                <th scope="col" className="num">
                  {t("admin.announcements.position")}
                </th>
                <th scope="col">{t("admin.announcements.expiresAt")}</th>
                <th scope="col">{t("admin.announcements.updated")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((a) => {
                const st = statusOf(a);
                return (
                  <tr key={a.id}>
                    <td>
                      <button type="button" className="link-btn" onClick={() => setEditing(a)}>
                        {a.title || stripHtml(a.content).slice(0, 80)}
                      </button>
                    </td>
                    <td>
                      <span className={`badge badge-${STATUS_BADGE[st]}`}>{t(`admin.announcements.statuses.${st}`)}</span>
                    </td>
                    <td>{t(`admin.announcements.placements.${a.placement}`)}</td>
                    <td className="num">{a.position}</td>
                    <td className="small">{a.expiresAt ? formatDateTime(a.expiresAt) : "—"}</td>
                    <td className="small">{formatDateTime(a.updatedAt)}</td>
                    <td className="row-actions">
                      <button type="button" className="icon-btn" onClick={() => setEditing(a)} aria-label={t("common.edit")} title={t("common.edit")}>
                        <FiEdit2 />
                      </button>
                      <button
                        type="button"
                        className="icon-btn"
                        onClick={() => toggle(a)}
                        aria-label={a.enabled ? t("admin.announcements.disable") : t("admin.announcements.enable")}
                        title={a.enabled ? t("admin.announcements.disable") : t("admin.announcements.enable")}
                      >
                        {a.enabled ? <FiEyeOff /> : <FiEye />}
                      </button>
                      <button type="button" className="icon-btn danger" onClick={() => setDeleting(a)} aria-label={t("common.delete")} title={t("common.delete")}>
                        <FiTrash2 />
                      </button>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("admin.announcements.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
      {editing && (
        <AnnouncementModal
          announcement={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null);
            reload();
          }}
        />
      )}
      <ConfirmModal
        show={!!deleting}
        title={t("common.delete")}
        body={t("admin.announcements.deleteBody", { title: deleting?.title || "" })}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          const a = deleting;
          setDeleting(null);
          try {
            await api.delete(`/v1/admin/announcements/${a.id}`);
            toast.success(t("admin.deleted"));
            reload();
          } catch (err) {
            toast.error(errMsg(err));
          }
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default Announcements;
