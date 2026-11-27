import { useEffect, useId, useRef } from "react";
import { FiChevronLeft, FiChevronRight, FiX } from "react-icons/fi";
import { FaStar, FaRegStar, FaStarHalfAlt } from "react-icons/fa";
import { useI18n } from "../i18n/I18nContext";

// Toggle is a switch built on a real checkbox (keyboard + screen reader friendly).
export const Toggle = ({ checked, onChange, label, disabled, hint, id }) => {
  const autoId = useId();
  const inputId = id || autoId;
  return (
    <label className={`toggle ${disabled ? "is-disabled" : ""}`} htmlFor={inputId}>
      <input
        id={inputId}
        type="checkbox"
        role="switch"
        checked={!!checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      <span className="toggle-track" aria-hidden="true">
        <span className="toggle-thumb" />
      </span>
      {label && <span className="toggle-label">{label}</span>}
      {hint && <span className="hint">{hint}</span>}
    </label>
  );
};

// Field wraps a single control with a real <label>.
export const Field = ({ label, hint, children, required, className = "", htmlFor }) => {
  const autoId = useId();
  const id = htmlFor || autoId;
  const child = typeof children === "function" ? children(id) : children;
  return (
    <div className={`field ${className}`}>
      {label && (
        <label htmlFor={id} className="field-label">
          {label}
          {required && <span className="required" aria-hidden="true"> *</span>}
        </label>
      )}
      {child}
      {hint && <p className="hint">{hint}</p>}
    </div>
  );
};

// Input/Select/Textarea bound to a Field id via render-prop children.
export const TextField = ({ label, hint, required, value, onChange, type = "text", className, ...rest }) => (
  <Field label={label} hint={hint} required={required} className={className}>
    {(id) => (
      <input
        id={id}
        className="input"
        type={type}
        value={value ?? ""}
        required={required}
        onChange={(e) => onChange(e.target.value)}
        {...rest}
      />
    )}
  </Field>
);

export const TextArea = ({ label, hint, required, value, onChange, rows = 4, className, ...rest }) => (
  <Field label={label} hint={hint} required={required} className={className}>
    {(id) => (
      <textarea
        id={id}
        className="input"
        rows={rows}
        value={value ?? ""}
        required={required}
        onChange={(e) => onChange(e.target.value)}
        {...rest}
      />
    )}
  </Field>
);

export const SelectField = ({ label, hint, required, value, onChange, options, className, placeholder, ...rest }) => (
  <Field label={label} hint={hint} required={required} className={className}>
    {(id) => (
      <select id={id} className="input" value={value ?? ""} required={required} onChange={(e) => onChange(e.target.value)} {...rest}>
        {placeholder !== undefined && <option value="">{placeholder}</option>}
        {options.map((o) => (
          <option key={String(o.value)} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    )}
  </Field>
);

export const Spinner = ({ label }) => {
  const { t } = useI18n();
  return (
    <div className="spinner-wrap" role="status" aria-live="polite">
      <span className="spinner" aria-hidden="true" />
      <span className="sr-only">{label || t("common.loading")}</span>
    </div>
  );
};

export const Empty = ({ title, body, children }) => (
  <div className="empty">
    {title && <strong>{title}</strong>}
    {body && <p>{body}</p>}
    {children}
  </div>
);

export const ErrorBox = ({ error }) => {
  const { errMsg } = useI18n();
  if (!error) return null;
  return (
    <div className="notice notice-error" role="alert">
      {typeof error === "string" ? error : errMsg(error)}
    </div>
  );
};

export const Modal = ({ show, title, onClose, children, footer, wide }) => {
  const ref = useRef(null);
  const closeRef = useRef(onClose);
  useEffect(() => {
    closeRef.current = onClose;
  });
  useEffect(() => {
    if (!show) return undefined;
    const onKey = (e) => e.key === "Escape" && closeRef.current?.();
    document.addEventListener("keydown", onKey);
    const prev = document.activeElement;
    ref.current?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      prev?.focus?.();
    };
  }, [show]);
  const { t } = useI18n();
  if (!show) return null;
  return (
    <div className="modal-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose?.()}>
      <div className={`modal ${wide ? "modal-wide" : ""}`} role="dialog" aria-modal="true" aria-label={title} tabIndex={-1} ref={ref}>
        <div className="modal-header">
          <h2>{title}</h2>
          <button type="button" className="icon-btn" onClick={onClose} aria-label={t("common.close")}>
            <FiX />
          </button>
        </div>
        <div className="modal-body">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  );
};

export const ConfirmModal = ({ show, title, body, confirmLabel, onConfirm, onCancel, danger = true }) => {
  const { t } = useI18n();
  return (
    <Modal
      show={show}
      title={title}
      onClose={onCancel}
      footer={
        <>
          <button type="button" className="btn btn-secondary" onClick={onCancel}>
            {t("common.cancel")}
          </button>
          <button type="button" className={`btn ${danger ? "btn-danger" : "btn-primary"}`} onClick={onConfirm}>
            {confirmLabel || t("common.confirm")}
          </button>
        </>
      }
    >
      <p>{body}</p>
    </Modal>
  );
};

export const Pagination = ({ page, totalPages, onChange }) => {
  const { t } = useI18n();
  if (!totalPages || totalPages <= 1) return null;
  const pages = [];
  const from = Math.max(1, page - 2);
  const to = Math.min(totalPages, page + 2);
  for (let p = from; p <= to; p++) pages.push(p);
  return (
    <nav className="pagination" aria-label={t("common.pagination")}>
      <button type="button" className="page-btn" disabled={page <= 1} onClick={() => onChange(page - 1)} aria-label={t("common.previous")}>
        <FiChevronLeft className="flip-rtl" />
      </button>
      {from > 1 && (
        <>
          <button type="button" className="page-btn" onClick={() => onChange(1)}>
            1
          </button>
          {from > 2 && <span className="page-gap">…</span>}
        </>
      )}
      {pages.map((p) => (
        <button
          key={p}
          type="button"
          className={`page-btn ${p === page ? "is-current" : ""}`}
          aria-current={p === page ? "page" : undefined}
          onClick={() => onChange(p)}
        >
          {p}
        </button>
      ))}
      {to < totalPages && (
        <>
          {to < totalPages - 1 && <span className="page-gap">…</span>}
          <button type="button" className="page-btn" onClick={() => onChange(totalPages)}>
            {totalPages}
          </button>
        </>
      )}
      <button type="button" className="page-btn" disabled={page >= totalPages} onClick={() => onChange(page + 1)} aria-label={t("common.next")}>
        <FiChevronRight className="flip-rtl" />
      </button>
    </nav>
  );
};

export const Stars = ({ rating = 0, count }) => {
  const { t } = useI18n();
  const r = Number(rating) || 0;
  const icons = [];
  for (let i = 1; i <= 5; i++) {
    if (r >= i) icons.push(<FaStar key={i} />);
    else if (r >= i - 0.5) icons.push(<FaStarHalfAlt key={i} className="flip-rtl" />);
    else icons.push(<FaRegStar key={i} />);
  }
  return (
    <span className="stars" title={t("product.ratedOutOf", { rating: r.toFixed(1) })}>
      <span aria-hidden="true">{icons}</span>
      <span className="sr-only">{t("product.ratedOutOf", { rating: r.toFixed(1) })}</span>
      {count !== undefined && <span className="stars-count">({count})</span>}
    </span>
  );
};

// StarInput is a radio group: keyboard navigable with arrow keys.
export const StarInput = ({ value, onChange, required }) => {
  const { t } = useI18n();
  const name = useId();
  return (
    <fieldset className="star-input">
      <legend className="field-label">
        {t("reviews.yourRating")}
        {required && <span className="required" aria-hidden="true"> *</span>}
      </legend>
      {[1, 2, 3, 4, 5].map((n) => (
        <label key={n} className={n <= value ? "is-on" : ""}>
          <input type="radio" name={name} value={n} checked={value === n} onChange={() => onChange(n)} required={required} />
          <FaStar aria-hidden="true" />
          <span className="sr-only">{t("reviews.stars", { n })}</span>
        </label>
      ))}
    </fieldset>
  );
};

export const Tabs = ({ tabs, active, onChange, className = "" }) => (
  <div className={`tabs ${className}`} role="tablist">
    {tabs.map((tab) => (
      <button
        key={tab.id}
        type="button"
        role="tab"
        aria-selected={active === tab.id}
        className={`tab ${active === tab.id ? "is-active" : ""}`}
        onClick={() => onChange(tab.id)}
      >
        {tab.label}
        {tab.count !== undefined && <span className="tab-count">{tab.count}</span>}
      </button>
    ))}
  </div>
);

export const StatusBadge = ({ status, label }) => <span className={`badge badge-${status}`}>{label || status}</span>;
