import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "react-toastify";
import { FiArrowRight, FiCheckCircle, FiCopy, FiDatabase, FiDownload, FiUpload, FiAlertTriangle } from "react-icons/fi";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { ErrorBox, Modal, SelectField, Spinner, TextField } from "../../components/ui";

// CloneData is "Clone data" (WordPress administrators): copy the main
// database to another one (export) or a remote one into the main one
// (import), MySQL/MariaDB ↔ PostgreSQL. GET/POST /v1/admin/clone,
// POST /v1/admin/clone/test, GET /v1/admin/clone/job.
const STORAGE_KEY = "cwmerch.cloneRemote";
const EMPTY_REMOTE = { type: "postgres", host: "", port: "", name: "", user: "", password: "", prefix: "wp_", sslMode: "disable" };

const loadRemote = () => {
  try {
    return { ...EMPTY_REMOTE, ...JSON.parse(localStorage.getItem(STORAGE_KEY) || "{}"), password: "" };
  } catch {
    return EMPTY_REMOTE;
  }
};

const saveRemote = (r) => {
  try {
    const { password: _password, ...rest } = r;
    localStorage.setItem(STORAGE_KEY, JSON.stringify(rest));
  } catch {
    /* private mode */
  }
};

const engineLabel = (type) => (type === "postgres" ? "PostgreSQL" : "MySQL / MariaDB");

const DbCard = ({ title, info, highlight }) => {
  const { t } = useI18n();
  if (!info) return null;
  return (
    <div className={`card stack db-card ${highlight ? "db-card-target" : ""}`}>
      <div className="db-card-head">
        <FiDatabase aria-hidden="true" />
        <div>
          <div className="small muted">{title}</div>
          <strong>{engineLabel(info.type)}</strong> <span className="muted small">{info.version}</span>
        </div>
      </div>
      <dl className="kv-list small">
        {info.host && (
          <div>
            <dt>{t("clone.database")}</dt>
            <dd dir="ltr">
              {info.host}/{info.name}
            </dd>
          </div>
        )}
        <div>
          <dt>{t("clone.prefix")}</dt>
          <dd dir="ltr">{info.prefix}</dd>
        </div>
        <div>
          <dt>{t("clone.migration")}</dt>
          <dd>{info.migration ? `V${info.migration}` : t("clone.noMigration")}</dd>
        </div>
        <div>
          <dt>{t("clone.content")}</dt>
          <dd>{t("clone.contentValue", { products: info.products, orders: info.orders, users: info.customers, rows: info.rows })}</dd>
        </div>
      </dl>
    </div>
  );
};

const statusClass = { done: "completed", failed: "failed", running: "processing", pending: "on-hold" };

const JobView = ({ job }) => {
  const { t } = useI18n();
  const [showEmpty, setShowEmpty] = useState(false);
  const visible = job.tables.filter((x) => showEmpty || x.sourceRows > 0 || x.beforeRows > 0 || x.status === "running" || x.status === "failed");
  const hidden = job.tables.length - visible.length;
  const pct = job.total ? Math.round((job.copied / Math.max(job.total, 1)) * 100) : job.status === "done" ? 100 : 0;
  const totalRows = job.tables.reduce((n, x) => n + (x.sourceRows || 0), 0);
  const copied = job.tables.reduce((n, x) => n + (x.copied || 0), 0);
  const livePct = totalRows ? Math.min(100, Math.round((copied / totalRows) * 100)) : pct;
  const doneTables = job.tables.filter((x) => x.status === "done").length;
  return (
    <section className="card stack">
      <div className="row-between">
        <h2>
          {job.direction === "import" ? t("clone.importTitle") : t("clone.exportTitle")}{" "}
          <span className={`badge badge-${statusClass[job.status] || "processing"}`}>{t(`clone.status.${job.status}`)}</span>
        </h2>
        <span className="muted small">{new Date(job.startedAt).toLocaleString()}</span>
      </div>
      <div className="clone-route small">
        <span>
          {engineLabel(job.source.type)} <span dir="ltr">{job.source.host ? `${job.source.host}/${job.source.name}` : ""}</span>
        </span>
        <FiArrowRight className="flip-rtl" aria-hidden="true" />
        <span>
          {engineLabel(job.target.type)} <span dir="ltr">{job.target.host ? `${job.target.host}/${job.target.name}` : ""}</span>
        </span>
        <span className="muted">· {t(`clone.conflict.${job.conflict || "overwrite"}`)}</span>
      </div>
      <div className="progress" role="progressbar" aria-valuenow={livePct} aria-valuemin={0} aria-valuemax={100}>
        <div className="progress-bar" style={{ width: `${livePct}%` }} />
      </div>
      <p className="small muted">
        {t("clone.progress", { tables: doneTables, totalTables: job.tables.length, rows: copied.toLocaleString(), total: totalRows.toLocaleString() })}
      </p>
      {job.error && <p className="notice notice-error">{job.error}</p>}
      <div className="table-wrap">
        <table className="table table-compact">
          <thead>
            <tr>
              <th>{t("clone.table")}</th>
              <th className="num">{t("clone.sourceRows")}</th>
              <th className="num">{t("clone.copied")}</th>
              <th className="num">{t("clone.replaced")}</th>
              <th className="num">{t("clone.before")}</th>
              <th className="num">{t("clone.after")}</th>
              <th>{t("clone.tableStatus")}</th>
            </tr>
          </thead>
          <tbody>
            {visible.map((x) => (
              <tr key={x.table}>
                <td dir="ltr">{x.table}</td>
                <td className="num">{x.sourceRows}</td>
                <td className="num">{x.copied}</td>
                <td className="num">{x.replaced || ""}</td>
                <td className="num">{x.status === "pending" ? "" : x.beforeRows}</td>
                <td className="num">{x.status === "done" ? x.afterRows : ""}</td>
                <td>
                  <span className={`badge badge-${statusClass[x.status]}`}>{t(`clone.status.${x.status}`)}</span>
                  {x.error && <div className="small text-danger">{x.error}</div>}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {(hidden > 0 || showEmpty) && (
        <div>
          <button type="button" className="btn btn-secondary btn-sm" onClick={() => setShowEmpty((v) => !v)}>
            {showEmpty ? t("clone.hideEmpty") : t("clone.showEmpty", { count: hidden })}
          </button>
        </div>
      )}
    </section>
  );
};

const CloneData = () => {
  const { t, errMsg } = useI18n();
  const [main, setMain] = useState(null);
  const [job, setJob] = useState(null);
  const [error, setError] = useState(null);
  const [direction, setDirection] = useState("export");
  const [conflict, setConflict] = useState("overwrite");
  const [remote, setRemote] = useState(loadRemote);
  const [test, setTest] = useState(null);
  const [testing, setTesting] = useState(false);
  const [confirm, setConfirm] = useState(false);
  const [typed, setTyped] = useState("");
  const timer = useRef(null);

  const poll = useCallback(() => {
    clearTimeout(timer.current);
    api
      .get("/v1/admin/clone/job")
      .then(({ data }) => {
        setJob(data);
        if (data.status === "running") timer.current = setTimeout(poll, 800);
        else api.get("/v1/admin/clone").then(({ data: d }) => setMain(d.main));
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    api
      .get("/v1/admin/clone")
      .then(({ data }) => {
        setMain(data.main);
        setJob(data.job);
        if (data.job?.status === "running") poll();
      })
      .catch(setError);
    return () => clearTimeout(timer.current);
  }, [poll]);

  const set = (k) => (v) => {
    setTest(null);
    setRemote((r) => ({ ...r, [k]: v }));
  };

  const runTest = async () => {
    setTesting(true);
    setTest(null);
    saveRemote(remote);
    try {
      const { data } = await api.post("/v1/admin/clone/test", { direction, remote });
      setTest(data);
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setTesting(false);
    }
  };

  const start = async () => {
    setConfirm(false);
    setTyped("");
    try {
      const { data } = await api.post("/v1/admin/clone", { direction, remote, conflict, confirm: true });
      setJob(data);
      toast.info(t("clone.started"));
      poll();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  if (!main) return error ? <ErrorBox error={error} /> : <Spinner />;
  const running = job?.status === "running";
  const destName = direction === "export" ? remote.name : main.name;
  const source = direction === "export" ? main : test?.remote;
  const target = direction === "export" ? test?.remote : main;

  return (
    <div className="stack-lg clone-page">
      <header className="page-header">
        <h1>
          <FiCopy aria-hidden="true" /> {t("clone.title")}
        </h1>
        <p className="muted">{t("clone.intro")}</p>
      </header>

      <div className="clone-directions" role="radiogroup" aria-label={t("clone.direction")}>
        {[
          ["export", FiUpload],
          ["import", FiDownload],
        ].map(([d, Icon]) => (
          <button
            key={d}
            type="button"
            role="radio"
            aria-checked={direction === d}
            className={`card clone-direction ${direction === d ? "is-active" : ""}`}
            onClick={() => {
              setDirection(d);
              setTest(null);
            }}
          >
            <Icon aria-hidden="true" />
            <strong>{t(`clone.${d}`)}</strong>
            <span className="small muted">{t(`clone.${d}Hint`)}</span>
          </button>
        ))}
      </div>

      <div className="clone-grid">
        <DbCard title={t("clone.mainDatabase")} info={main} highlight={direction === "import"} />
        <section className="card stack">
          <h2>{direction === "export" ? t("clone.targetDatabase") : t("clone.sourceDatabase")}</h2>
          <SelectField
            label={t("clone.engine")}
            value={remote.type}
            onChange={set("type")}
            options={[
              { value: "postgres", label: "PostgreSQL" },
              { value: "mysql", label: "MySQL" },
              { value: "mariadb", label: "MariaDB" },
            ]}
          />
          <div className="form-grid">
            <TextField label={t("clone.host")} dir="ltr" value={remote.host} onChange={set("host")} required placeholder="db.example.com" />
            <TextField label={t("clone.port")} dir="ltr" value={remote.port} onChange={set("port")} placeholder={remote.type === "postgres" ? "5432" : "3306"} />
          </div>
          <div className="form-grid">
            <TextField label={t("clone.name")} dir="ltr" value={remote.name} onChange={set("name")} required />
            <TextField label={t("clone.prefix")} dir="ltr" value={remote.prefix} onChange={set("prefix")} placeholder="wp_" />
          </div>
          <div className="form-grid">
            <TextField label={t("clone.user")} dir="ltr" value={remote.user} onChange={set("user")} autoComplete="off" />
            <TextField label={t("clone.password")} dir="ltr" type="password" value={remote.password} onChange={set("password")} autoComplete="new-password" />
          </div>
          {remote.type === "postgres" && (
            <SelectField
              label={t("clone.sslMode")}
              value={remote.sslMode}
              onChange={set("sslMode")}
              options={["disable", "prefer", "require", "verify-full"].map((v) => ({ value: v, label: v }))}
            />
          )}
          <div>
            <button type="button" className="btn btn-secondary" onClick={runTest} disabled={testing || !remote.host || !remote.name}>
              {testing ? t("clone.testing") : t("clone.test")}
            </button>
          </div>
        </section>
      </div>

      {test && (
        <section className="card stack">
          <h2>{t("clone.check")}</h2>
          <div className="clone-compare">
            <DbCard title={t("clone.source")} info={source} />
            <FiArrowRight className="clone-arrow flip-rtl" aria-hidden="true" />
            <DbCard title={t("clone.destination")} info={target} highlight />
          </div>
          {test.ready ? (
            <p className="notice notice-success">
              <FiCheckCircle aria-hidden="true" /> {t("clone.ready")}
            </p>
          ) : (
            <div className="notice notice-error">
              <strong>{t("clone.notReady")}</strong>
              <ul>
                {(test.issues || []).map((x) => (
                  <li key={x}>{x}</li>
                ))}
              </ul>
            </div>
          )}
          {test.missing?.length > 0 && <p className="small muted">{t("clone.missing", { tables: test.missing.join(", ") })}</p>}
          <SelectField
            label={t("clone.conflictLabel")}
            hint={t(`clone.conflictHint.${conflict}`)}
            value={conflict}
            onChange={setConflict}
            options={["overwrite", "skip"].map((v) => ({ value: v, label: t(`clone.conflict.${v}`) }))}
          />
          <p className="notice notice-info small">
            <FiAlertTriangle aria-hidden="true" /> {t("clone.backupNotice")}
          </p>
          <div>
            <button type="button" className="btn btn-danger" disabled={!test.ready || running} onClick={() => setConfirm(true)}>
              <FiCopy aria-hidden="true" /> {t("clone.cloneButton")}
            </button>
          </div>
        </section>
      )}

      {job && <JobView job={job} />}

      <Modal
        show={confirm}
        title={t("clone.confirmTitle")}
        onClose={() => setConfirm(false)}
        footer={
          <>
            <button type="button" className="btn btn-secondary" onClick={() => setConfirm(false)}>
              {t("common.cancel")}
            </button>
            <button type="button" className="btn btn-danger" disabled={typed.trim() !== destName} onClick={start}>
              {t("clone.cloneButton")}
            </button>
          </>
        }
      >
        <div className="stack">
          <p>
            {t(direction === "export" ? "clone.confirmExport" : "clone.confirmImport", {
              name: destName,
              mode: t(`clone.conflict.${conflict}`),
            })}
          </p>
          <TextField label={t("clone.typeName", { name: destName })} dir="ltr" value={typed} onChange={setTyped} autoComplete="off" />
        </div>
      </Modal>
    </div>
  );
};

export default CloneData;
