import { useI18n } from "../i18n/I18nContext";
import { daysAgo, isoDay } from "../lib/util";

const presets = () => {
  const now = new Date();
  const today = isoDay(now);
  return [
    { id: "7d", from: daysAgo(6), to: today },
    { id: "30d", from: daysAgo(29), to: today },
    { id: "month", from: isoDay(new Date(now.getFullYear(), now.getMonth(), 1)), to: today },
    { id: "lastMonth", from: isoDay(new Date(now.getFullYear(), now.getMonth() - 1, 1)), to: isoDay(new Date(now.getFullYear(), now.getMonth(), 0)) },
    { id: "year", from: isoDay(new Date(now.getFullYear(), 0, 1)), to: today },
  ];
};

export const defaultRange = () => ({ from: daysAgo(29), to: isoDay(new Date()) });

const DateRange = ({ value, onChange, children }) => {
  const { t } = useI18n();
  return (
    <div className="date-range">
      <div className="date-range-presets" role="group" aria-label={t("range.presets")}>
        {presets().map((p) => (
          <button
            key={p.id}
            type="button"
            className={`chip ${value.from === p.from && value.to === p.to ? "is-active" : ""}`}
            onClick={() => onChange({ from: p.from, to: p.to })}
          >
            {t(`range.${p.id}`)}
          </button>
        ))}
      </div>
      <label className="inline-field">
        <span>{t("range.from")}</span>
        <input className="input input-sm" type="date" value={value.from} max={value.to} onChange={(e) => onChange({ ...value, from: e.target.value })} />
      </label>
      <label className="inline-field">
        <span>{t("range.to")}</span>
        <input className="input input-sm" type="date" value={value.to} min={value.from} onChange={(e) => onChange({ ...value, to: e.target.value })} />
      </label>
      {children}
    </div>
  );
};

export default DateRange;
