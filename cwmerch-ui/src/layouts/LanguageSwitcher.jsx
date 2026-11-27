import { useId } from "react";
import { useI18n } from "../i18n/I18nContext";

const LanguageSwitcher = ({ className = "" }) => {
  const { locale, setLocale, languages, t } = useI18n();
  const id = useId();
  // Only the languages enabled in the shop settings; nothing to switch with one.
  if (languages.length < 2) return null;
  return (
    <span className={`lang-switch ${className}`}>
      <label htmlFor={id} className="sr-only">
        {t("common.language")}
      </label>
      <select id={id} className="input input-sm" value={locale} onChange={(e) => setLocale(e.target.value)}>
        {languages.map((l) => (
          <option key={l.code} value={l.code}>
            {l.label}
          </option>
        ))}
      </select>
    </span>
  );
};

export default LanguageSwitcher;
