import { useState } from "react";
import { Navigate } from "react-router-dom";
import { toast } from "react-toastify";
import { FiEdit2, FiGlobe, FiShieldOff, FiTrash2 } from "react-icons/fi";
import { api, asArray, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { ROLES } from "../../lib/util";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox, SummaryCards } from "../../components/admin";
import { ConfirmModal, ErrorBox, Modal, Pagination, Spinner, TextField } from "../../components/ui";

const geoLabel = (geo) => {
  if (!geo) return "";
  const label = [geo.city, geo.region, geo.country].filter(Boolean).join(", ");
  return geo.countryCode ? (label ? `${label} (${geo.countryCode})` : geo.countryCode) : label;
};

export const SuperuserOnly = ({ children }) => {
  const { isSuperuser } = useAuth();
  return isSuperuser ? children : <Navigate to="/admin" replace />;
};

const UserIps = ({ user, onClose }) => {
  const { t, formatDateTime } = useI18n();
  const { data, loading, error } = useAsync(async () => asArray((await api.get(`/v1/admin/users/${user.id}/ips`)).data), [user.id], []);
  return (
    <Modal show title={t("users.ipsOf", { email: user.email })} onClose={onClose} wide>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : data.length === 0 ? (
        <p className="muted">{t("users.noIps")}</p>
      ) : (
        <div className="table-wrap">
          <table className="table">
            <thead>
              <tr>
                <th scope="col">IP</th>
                <th scope="col">{t("ips.location")}</th>
                <th scope="col" className="num">{t("ips.count")}</th>
                <th scope="col">{t("ips.lastSeen")}</th>
                <th scope="col">{t("ips.status")}</th>
              </tr>
            </thead>
            <tbody>
              {data.map((r) => (
                <tr key={r.ip}>
                  <td>
                    <code>{r.ip}</code>
                  </td>
                  <td className="small">{geoLabel(r.geo) || "—"}</td>
                  <td className="num">{r.count}</td>
                  <td className="small">{formatDateTime(r.lastSeen)}</td>
                  <td>{r.banned ? <span className="badge badge-failed">{t("ips.banned")}</span> : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Modal>
  );
};

// EditUser: an administrator edits a user's email, names, role and password
// (PUT /v1/admin/users/{id}); the email changes at once (no confirmation,
// unlike a user changing their own).
const EditUser = ({ user, self, onClose, onSaved }) => {
  const { t, errMsg } = useI18n();
  const [form, setForm] = useState({ email: user.email || "", name: user.name || "", surname: user.surname || "", role: user.role, password: "" });
  const [mfaOn, setMfaOn] = useState(!!user.mfaEnabled);
  const [confirmMfa, setConfirmMfa] = useState(false);

  // Removes every MFA method (TOTP, passkeys) of the user.
  const removeMfa = async () => {
    setConfirmMfa(false);
    try {
      await api.post(`/v1/admin/users/${user.id}/disable-mfa`, {});
      setMfaOn(false);
      toast.success(t("users.mfaDisabled"));
      onSaved(false);
    } catch (err) {
      setError(errMsg(err));
    }
  };
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body = { email: form.email.trim(), name: form.name.trim(), surname: form.surname.trim() };
      if (!self && form.role !== user.role) body.role = form.role;
      if (form.password) body.password = form.password;
      await api.put(`/v1/admin/users/${user.id}`, body);
      toast.success(t("users.saved"));
      onSaved(true);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal show title={t("users.editTitle", { email: user.email })} onClose={onClose}>
      <form className="stack" onSubmit={save}>
        <TextField label={t("auth.email")} type="email" value={form.email} onChange={set("email")} required autoComplete="off" />
        <div className="form-grid">
          <TextField label={t("auth.firstName")} value={form.name} onChange={set("name")} autoComplete="off" />
          <TextField label={t("auth.lastName")} value={form.surname} onChange={set("surname")} autoComplete="off" />
        </div>
        <label className="field">
          <span className="field-label">{t("users.role")}</span>
          <select className="input" value={form.role} onChange={(e) => set("role")(e.target.value)} disabled={self}>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {t(`roles.${r}`)}
              </option>
            ))}
          </select>
          {self && <span className="hint">{t("users.ownRole")}</span>}
        </label>
        <TextField label={t("auth.newPassword")} hint={t("account.passwordBlank")} type="password" value={form.password} onChange={set("password")} autoComplete="new-password" />
        <fieldset className="fieldset stack">
          <legend>MFA</legend>
          {mfaOn ? (
            <div className="row wrap">
              <span className="grow">
                <span className="badge badge-completed">{t("common.on")}</span>
              </span>
              <button type="button" className="btn btn-secondary btn-sm danger" onClick={() => setConfirmMfa(true)}>
                <FiShieldOff aria-hidden="true" /> {t("users.disableMfa")}
              </button>
            </div>
          ) : (
            <p className="muted small">{t("users.mfaNone")}</p>
          )}
        </fieldset>
        <ErrorBox error={error} />
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy || !form.email.trim()}>
            {t("common.saveChanges")}
          </button>
        </div>
      </form>
      <ConfirmModal
        show={confirmMfa}
        title={t("users.disableMfa")}
        body={t("users.disableMfaBody", { email: user.email })}
        confirmLabel={t("users.disableMfa")}
        onConfirm={removeMfa}
        onCancel={() => setConfirmMfa(false)}
      />
    </Modal>
  );
};

export const Users = () => {
  const { t, errMsg, formatDate } = useI18n();
  const { user: me } = useAuth();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [role, setRole] = useState("");
  const [deleting, setDeleting] = useState(null);
  const [mfaOff, setMfaOff] = useState(null);
  const [ips, setIps] = useState(null);
  const [editing, setEditing] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/users", { params: { page, perPage: 20, search: search || undefined, role: role || undefined } })).data, page),
    [page, search, role],
    { items: [], totalPages: 1 }
  );

  const run = async (fn, ok) => {
    try {
      await fn();
      if (ok) toast.success(ok);
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.users")} />
      <div className="toolbar">
        <label className="inline-field">
          <span>{t("users.role")}</span>
          <select
            className="input input-sm"
            value={role}
            onChange={(e) => {
              setRole(e.target.value);
              setPage(1);
            }}
          >
            <option value="">{t("common.all")}</option>
            {ROLES.map((r) => (
              <option key={r} value={r}>
                {t(`roles.${r}`)}
              </option>
            ))}
          </select>
        </label>
        <SearchBox
          value={search}
          onSearch={(s) => {
            setSearch(s);
            setPage(1);
          }}
          placeholder={t("users.search")}
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
                <th scope="col">{t("auth.email")}</th>
                <th scope="col">{t("admin.customers.username")}</th>
                <th scope="col">{t("admin.customers.name")}</th>
                <th scope="col">{t("users.role")}</th>
                <th scope="col">MFA</th>
                <th scope="col">{t("users.created")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((u) => (
                <tr key={u.id}>
                  <td>{u.email}</td>
                  <td>{u.username}</td>
                  <td>{u.displayName || [u.name, u.surname].filter(Boolean).join(" ")}</td>
                  <td>
                    <label className="sr-only" htmlFor={`role-${u.id}`}>
                      {t("users.roleOf", { email: u.email })}
                    </label>
                    <select
                      id={`role-${u.id}`}
                      className="input input-sm"
                      value={u.role}
                      disabled={u.id === me?.id}
                      onChange={(e) => run(() => api.put(`/v1/admin/users/${u.id}`, { role: e.target.value }), t("users.roleChanged"))}
                    >
                      {ROLES.map((r) => (
                        <option key={r} value={r}>
                          {t(`roles.${r}`)}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>{u.mfaEnabled ? <span className="badge badge-completed">{t("common.on")}</span> : <span className="muted">—</span>}</td>
                  <td className="small">{formatDate(u.createdAt)}</td>
                  <td className="row-actions">
                    <button type="button" className="icon-btn" onClick={() => setEditing(u)} aria-label={t("users.edit")} title={t("users.edit")}>
                      <FiEdit2 />
                    </button>
                    <button type="button" className="icon-btn" onClick={() => setIps(u)} aria-label={t("users.ips")} title={t("users.ips")}>
                      <FiGlobe />
                    </button>
                    {u.mfaEnabled && (
                      <button type="button" className="icon-btn" onClick={() => setMfaOff(u)} aria-label={t("users.disableMfa")} title={t("users.disableMfa")}>
                        <FiShieldOff />
                      </button>
                    )}
                    {u.id !== me?.id && (
                      <button type="button" className="icon-btn danger" onClick={() => setDeleting(u)} aria-label={t("common.delete")} title={t("common.delete")}>
                        <FiTrash2 />
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("users.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
      {ips && <UserIps user={ips} onClose={() => setIps(null)} />}
      {editing && (
        <EditUser
          user={editing}
          self={editing.id === me?.id}
          onClose={() => setEditing(null)}
          onSaved={(close) => {
            if (close) setEditing(null);
            reload();
          }}
        />
      )}
      <ConfirmModal
        show={!!mfaOff}
        title={t("users.disableMfa")}
        body={mfaOff ? t("users.disableMfaBody", { email: mfaOff.email }) : ""}
        confirmLabel={t("users.disableMfa")}
        onConfirm={() => {
          const u = mfaOff;
          setMfaOff(null);
          run(() => api.post(`/v1/admin/users/${u.id}/disable-mfa`, {}), t("users.mfaDisabled"));
        }}
        onCancel={() => setMfaOff(null)}
      />
      <ConfirmModal
        show={!!deleting}
        title={t("users.deleteTitle")}
        body={deleting ? t("users.deleteBody", { email: deleting.email }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={() => {
          const u = deleting;
          setDeleting(null);
          run(() => api.delete(`/v1/admin/users/${u.id}`), t("admin.deleted"));
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export const Audit = () => {
  const { t, formatDateTime } = useI18n();
  const [page, setPage] = useState(1);
  const [action, setAction] = useState("");
  const [userId, setUserId] = useState("");
  const { data: raw, loading, error } = useAsync(async () => (await api.get("/v1/admin/audit", { params: { page, perPage: 50, action: action || undefined, userId: userId || undefined } })).data, [page, action, userId]);
  const data = normalizePaged(raw, page, 50);
  const actions = raw?.actions || [];
  const detail = (d) => (d && typeof d === "object" ? Object.entries(d).map(([k, v]) => `${k}: ${typeof v === "object" ? JSON.stringify(v) : v}`).join(" · ") : d || "");
  return (
    <div className="stack">
      <PageHead title={t("admin.nav.audit")} subtitle={t("audit.intro")} />
      <div className="toolbar">
        <div className="row wrap">
          {actions.length > 0 ? (
            <label className="inline-field">
              <span>{t("audit.action")}</span>
              <select
                className="input input-sm"
                value={action}
                onChange={(e) => {
                  setAction(e.target.value);
                  setPage(1);
                }}
              >
                <option value="">{t("common.all")}</option>
                {actions.map((a) => (
                  <option key={a} value={a}>
                    {a}
                  </option>
                ))}
              </select>
            </label>
          ) : (
            <SearchBox
              value={action}
              label={t("audit.action")}
              placeholder={t("audit.action")}
              onSearch={(s) => {
                setAction(s);
                setPage(1);
              }}
            />
          )}
          <form
            className="row"
            onSubmit={(e) => {
              e.preventDefault();
              setUserId(new FormData(e.currentTarget).get("uid") || "");
              setPage(1);
            }}
          >
            <label className="sr-only" htmlFor="audit-uid">
              {t("audit.userId")}
            </label>
            <input id="audit-uid" name="uid" className="input input-sm" placeholder={t("audit.userId")} defaultValue={userId} />
            <button type="submit" className="btn btn-secondary btn-sm">
              {t("shop.filter")}
            </button>
          </form>
        </div>
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table">
            <thead>
              <tr>
                <th scope="col">{t("audit.when")}</th>
                <th scope="col">{t("audit.action")}</th>
                <th scope="col">{t("audit.actor")}</th>
                <th scope="col">{t("audit.subject")}</th>
                <th scope="col">{t("audit.detail")}</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((e) => (
                <tr key={e.id}>
                  <td className="small nowrap">{formatDateTime(e.createdAt)}</td>
                  <td>
                    <code>{e.action}</code>
                  </td>
                  <td className="small">{e.userEmail || e.userId || "—"}</td>
                  <td className="small">
                    {e.subjectType} {e.subjectId && <code>{e.subjectId}</code>}
                  </td>
                  <td className="small">{detail(e.detail)}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("audit.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </div>
  );
};

export const IpBans = () => {
  const { t, errMsg, formatDateTime } = useI18n();
  const [page, setPage] = useState(1);
  const [term, setTerm] = useState("");
  const [banOpen, setBanOpen] = useState(false);
  const [ban, setBan] = useState({ ip: "", motif: "" });
  const [unbanning, setUnbanning] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/ip-bans/", { params: { page, term: term || undefined, search: term || undefined } })).data, page),
    [page, term],
    { items: [], totalPages: 1 }
  );
  const { data: stats, reload: reloadStats } = useAsync(async () => (await api.get("/v1/admin/ip-bans/stats")).data, [], { total: 0, banned: 0 });

  const put = async (body) => {
    try {
      await api.put("/v1/admin/ip-bans/", body);
      toast.success(body.action === "ban" ? t("ips.bannedToast") : t("ips.unbannedToast"));
      reload();
      reloadStats();
      return true;
    } catch (err) {
      toast.error(errMsg(err));
      return false;
    }
  };

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.ipBans")} subtitle={t("ips.intro")}>
        <button type="button" className="btn btn-danger btn-sm" onClick={() => setBanOpen(true)}>
          {t("ips.banAddress")}
        </button>
      </PageHead>
      <SummaryCards
        cards={[
          { label: t("ips.total"), value: stats?.total ?? 0 },
          { label: t("ips.bannedCount"), value: stats?.banned ?? 0 },
        ]}
      />
      <div className="toolbar">
        <span />
        <SearchBox
          value={term}
          onSearch={(s) => {
            setTerm(s);
            setPage(1);
          }}
          placeholder={t("ips.filter")}
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
                <th scope="col">IP</th>
                <th scope="col">{t("ips.members")}</th>
                <th scope="col">{t("ips.location")}</th>
                <th scope="col" className="num">{t("ips.count")}</th>
                <th scope="col">{t("ips.lastSeen")}</th>
                <th scope="col">{t("ips.status")}</th>
                <th scope="col">
                  <span className="sr-only">{t("common.actions")}</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((r) => (
                <tr key={r.ip}>
                  <td>
                    <code>{r.ip}</code>
                  </td>
                  <td className="small">{(r.members || []).map((m) => m.email).join(", ") || t("ips.anonymous")}</td>
                  <td className="small">{geoLabel(r.geo) || "—"}</td>
                  <td className="num">{r.count ?? 0}</td>
                  <td className="small">{r.lastSeen ? formatDateTime(r.lastSeen) : "—"}</td>
                  <td>{r.banned ? <span className="badge badge-failed" title={r.reason || r.motif}>{t("ips.banned")}</span> : "—"}</td>
                  <td>
                    {r.banned ? (
                      <button type="button" className="btn btn-secondary btn-sm" onClick={() => setUnbanning(r.ip)}>
                        {t("ips.unban")}
                      </button>
                    ) : (
                      <button type="button" className="btn btn-danger btn-sm" onClick={() => put({ ip: r.ip, action: "ban", motif: "" })}>
                        {t("ips.ban")}
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("ips.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
      <Modal show={banOpen} title={t("ips.banAddress")} onClose={() => setBanOpen(false)}>
        <form
          className="stack"
          onSubmit={async (e) => {
            e.preventDefault();
            if (await put({ ip: ban.ip.trim(), action: "ban", motif: ban.motif.trim() })) {
              setBanOpen(false);
              setBan({ ip: "", motif: "" });
            }
          }}
        >
          <TextField label={t("ips.address")} value={ban.ip} onChange={(ip) => setBan({ ...ban, ip })} required />
          <TextField label={t("ips.motif")} value={ban.motif} onChange={(motif) => setBan({ ...ban, motif })} />
          <div className="row end">
            <button type="submit" className="btn btn-danger" disabled={!ban.ip.trim()}>
              {t("ips.ban")}
            </button>
          </div>
        </form>
      </Modal>
      <ConfirmModal
        show={!!unbanning}
        title={t("ips.unban")}
        body={t("ips.unbanBody", { ip: unbanning || "" })}
        confirmLabel={t("ips.unban")}
        danger={false}
        onConfirm={() => {
          const ip = unbanning;
          setUnbanning(null);
          put({ ip, action: "unban" });
        }}
        onCancel={() => setUnbanning(null)}
      />
    </div>
  );
};
