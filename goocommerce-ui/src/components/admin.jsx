import { useState } from "react";
import { FiSearch } from "react-icons/fi";
import { useI18n } from "../i18n/I18nContext";

export const PageHead = ({ title, children, subtitle }) => (
  <div className="admin-head">
    <div>
      <h1>{title}</h1>
      {subtitle && <p className="muted">{subtitle}</p>}
    </div>
    {children && <div className="admin-head-actions">{children}</div>}
  </div>
);

// SearchBox submits on Enter / button (not on every keystroke).
export const SearchBox = ({ value, onSearch, placeholder, label }) => {
  const { t } = useI18n();
  const [draft, setDraft] = useState(value || "");
  return (
    <form
      className="search-box"
      role="search"
      onSubmit={(e) => {
        e.preventDefault();
        onSearch(draft.trim());
      }}
    >
      <label className="sr-only" htmlFor="admin-search">
        {label || t("common.search")}
      </label>
      <input id="admin-search" className="input input-sm" type="search" value={draft} placeholder={placeholder || t("common.search")} onChange={(e) => setDraft(e.target.value)} />
      <button type="submit" className="btn btn-secondary btn-sm" aria-label={t("common.search")}>
        <FiSearch aria-hidden="true" />
      </button>
    </form>
  );
};

export const SummaryCards = ({ cards }) => (
  <div className="summary-cards">
    {cards.map((c) => (
      <div key={c.label} className="summary-card">
        <span className="summary-label">{c.label}</span>
        <strong className="summary-value">{c.value}</strong>
      </div>
    ))}
  </div>
);

// GenericTable renders rows of unknown shape (e.g. accounting JSON): columns
// are the keys of the first row unless given.
export const GenericTable = ({ rows, columns, format = (v) => v }) => {
  const list = rows || [];
  if (list.length === 0) return null;
  const cols = columns || Object.keys(list[0]).filter((k) => typeof list[0][k] !== "object" || list[0][k] === null);
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {cols.map((c) => (
              <th key={c} scope="col" className={typeof list[0][c] === "number" ? "num" : ""}>
                {c}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {list.map((r, i) => (
            <tr key={i}>
              {cols.map((c) => (
                <td key={c} className={typeof r[c] === "number" ? "num" : ""}>
                  {format(r[c], c)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

// ImageUpload posts a file to /v1/admin/media and returns {id, src, name, alt}.
export const uploadMedia = async (api, file) => {
  const fd = new FormData();
  fd.append("file", file);
  const { data } = await api.post("/v1/admin/media", fd, { headers: { "Content-Type": "multipart/form-data" } });
  return data;
};

export const UploadButton = ({ onFiles, label, multiple, accept = "image/*", disabled }) => (
  <label className={`btn btn-secondary btn-sm upload-btn ${disabled ? "is-disabled" : ""}`}>
    <input
      type="file"
      accept={accept}
      multiple={multiple}
      disabled={disabled}
      className="sr-only"
      onChange={(e) => {
        const files = Array.from(e.target.files || []);
        e.target.value = "";
        if (files.length) onFiles(files);
      }}
    />
    {label}
  </label>
);
