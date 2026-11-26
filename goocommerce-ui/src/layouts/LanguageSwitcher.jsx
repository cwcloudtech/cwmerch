import { useId } from "react";
import { LANGUAGES, useI18n } from "../i18n/I18nContext";

const LanguageSwitcher = ({ className = "" }) => {
  const { locale, setLocale, t } = useI18n();
  const id = useId();
  return (
    <span className={`lang-switch ${className}`}>
      <label htmlFor={id} className="sr-only">
        {t("common.language")}
      </label>
      <select id={id} className="input input-sm" value={locale} onChange={(e) => setLocale(e.target.value)}>
        {LANGUAGES.map((l) => (
          <option key={l.code} value={l.code}>
            {l.label}
          </option>
        ))}
      </select>
    </span>
  );
};

export default LanguageSwitcher;
