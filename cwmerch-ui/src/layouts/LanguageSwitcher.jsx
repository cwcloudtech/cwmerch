import { useEffect, useId, useRef, useState } from "react";
import { FiCheck, FiChevronDown, FiGlobe } from "react-icons/fi";
import { useI18n } from "../i18n/I18nContext";

// Like cwclock: each language with its flag.
const FLAGS = { en: "🇬🇧", fr: "🇫🇷", ar: "🇹🇳" };

const LanguageSwitcher = ({ className = "" }) => {
  const { locale, setLocale, languages, t } = useI18n();
  const [open, setOpen] = useState(false);
  const ref = useRef(null);
  const menuId = useId();

  useEffect(() => {
    if (!open) return undefined;
    const onDown = (e) => ref.current && !ref.current.contains(e.target) && setOpen(false);
    const onKey = (e) => e.key === "Escape" && setOpen(false);
    document.addEventListener("pointerdown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  // Only the languages enabled in the shop settings; nothing to switch with one.
  if (languages.length < 2) return null;
  const current = languages.find((l) => l.code === locale) || languages[0];

  return (
    <div className={`lang-switch ${open ? "is-open" : ""} ${className}`} ref={ref}>
      <button
        type="button"
        className="lang-switch-btn"
        aria-haspopup="true"
        aria-expanded={open}
        aria-controls={menuId}
        aria-label={`${t("common.language")}: ${current.label}`}
        title={t("common.language")}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="lang-flag" aria-hidden="true">
          {FLAGS[current.code] || <FiGlobe />}
        </span>
        <span className="lang-code" aria-hidden="true">
          {current.code.toUpperCase()}
        </span>
        <FiChevronDown className="lang-chevron" aria-hidden="true" />
      </button>
      {open && (
        <ul className="lang-menu" id={menuId} role="menu">
          <li className="lang-menu-title" role="presentation">
            <FiGlobe aria-hidden="true" /> {t("common.language")}
          </li>
          {languages.map((l) => (
            <li key={l.code} role="none">
              <button
                type="button"
                role="menuitemradio"
                aria-checked={l.code === locale}
                className={l.code === locale ? "is-active" : ""}
                lang={l.code}
                onClick={() => {
                  setLocale(l.code);
                  setOpen(false);
                }}
              >
                <span className="lang-flag" aria-hidden="true">
                  {FLAGS[l.code] || <FiGlobe />}
                </span>
                <span className="lang-label">{l.label}</span>
                {l.code === locale && <FiCheck className="lang-check" aria-hidden="true" />}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
};

export default LanguageSwitcher;
