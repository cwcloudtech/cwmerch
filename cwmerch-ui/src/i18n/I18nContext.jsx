import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import en from "./en.json";
import fr from "./fr.json";
import ar from "./ar.json";

export const dictionaries = { en, fr, ar };
export const LANGUAGES = [
  { code: "en", label: "English" },
  { code: "fr", label: "Français" },
  { code: "ar", label: "العربية" },
];
const RTL_LOCALES = new Set(["ar"]);
export const isRtl = (locale) => RTL_LOCALES.has(locale);
export const dirFor = (locale) => (isRtl(locale) ? "rtl" : "ltr");

const STORAGE_KEY = "cwmerch.locale";
// The shop's language config ({languages, defaultLanguage}) is cached so the
// next visit resolves the right language before /v1/shop/settings answers.
const CONFIG_KEY = "cwmerch.languages";
const ARABIC_FONT_HREF = "https://fonts.googleapis.com/css2?family=Noto+Sans+Arabic:wght@400;600;700&display=swap";

const readStorage = (key) => {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
};
const writeStorage = (key, value) => {
  try {
    localStorage.setItem(key, value);
  } catch {
    /* storage unavailable */
  }
};

// normalizeConfig keeps only known languages (at least one) and a default
// that is among them.
export const normalizeConfig = (cfg) => {
  const known = LANGUAGES.map((l) => l.code);
  let languages = Array.isArray(cfg?.languages) ? known.filter((c) => cfg.languages.includes(c)) : [];
  if (languages.length === 0) languages = known;
  const defaultLanguage = languages.includes(cfg?.defaultLanguage) ? cfg.defaultLanguage : languages.includes("en") ? "en" : languages[0];
  return { languages, defaultLanguage };
};

const cachedConfig = () => {
  try {
    return normalizeConfig(JSON.parse(readStorage(CONFIG_KEY) || "null"));
  } catch {
    return normalizeConfig(null);
  }
};

const browserLanguages = () => {
  if (typeof navigator === "undefined") return [];
  const list = navigator.languages && navigator.languages.length ? navigator.languages : [navigator.language || ""];
  return list.map((l) => String(l || "").slice(0, 2).toLowerCase());
};

// pickLocale: a stored choice wins when still enabled (else the shop default);
// without one, the first enabled browser language, else the shop default.
const pickLocale = (cfg, current) => {
  const stored = readStorage(STORAGE_KEY);
  if (stored && dictionaries[stored]) return cfg.languages.includes(stored) ? stored : cfg.defaultLanguage;
  if (current && cfg.languages.includes(current)) return current;
  const browser = browserLanguages().find((l) => cfg.languages.includes(l));
  return browser || cfg.defaultLanguage;
};

const resolve = (dict, key) =>
  key.split(".").reduce((acc, part) => (acc && acc[part] !== undefined ? acc[part] : undefined), dict);

export const translate = (locale, key, vars) => {
  let str = resolve(dictionaries[locale], key) ?? resolve(dictionaries.en, key);
  if (typeof str !== "string") str = key;
  if (vars) {
    // In right-to-left text each value is bidi-isolated (FSI…PDI) so phone
    // numbers, e-mails, Latin names and amounts keep their own order.
    const rtl = isRtl(locale);
    Object.keys(vars).forEach((k) => {
      const v = vars[k] ?? "";
      str = str.split(`{{${k}}}`).join(rtl && v !== "" ? `\u2068${v}\u2069` : v);
    });
  }
  return str;
};

// errorMessage turns an axios error into a user-facing string: translated
// i18n_code when we know it, else the API message; a 5xx gets the generic
// message plus the X-Cid correlation id.
export const errorMessage = (error, locale) => {
  const status = Number(error?.response?.status || 0);
  const data = error?.response?.data;
  const code = data && typeof data === "object" ? data.i18n_code : null;
  const known = code ? resolve(dictionaries[locale], code) ?? resolve(dictionaries.en, code) : undefined;
  if (status >= 500) {
    const cid = error?.response?.headers?.["x-cid"];
    const base = typeof known === "string" ? known : translate(locale, "errors.server");
    return cid ? `${base} (${translate(locale, "errors.ref", { cid })})` : base;
  }
  if (typeof known === "string") return known;
  if (data && typeof data === "object" && data.message) return data.message;
  if (!error?.response) return translate(locale, "errors.network");
  return translate(locale, "errors.generic");
};

// intlLocale: the BCP 47 tag for Intl date formatting. Arabic keeps Western
// digits so dates read consistently with the shop's money format.
const intlLocale = (locale) => (locale === "ar" ? "ar-u-nu-latn" : locale);

const loadArabicFont = () => {
  const id = "cwmerch-arabic-font";
  if (document.getElementById(id)) return;
  const link = document.createElement("link");
  link.id = id;
  link.rel = "stylesheet";
  link.href = ARABIC_FONT_HREF;
  document.head.appendChild(link);
};

const I18nContext = createContext(null);

export const I18nProvider = ({ children }) => {
  const [config, setConfig] = useState(cachedConfig);
  const [locale, setLocaleState] = useState(() => pickLocale(cachedConfig(), null));

  useEffect(() => {
    const root = document.documentElement;
    root.lang = locale;
    root.dir = dirFor(locale);
    if (locale === "ar") loadArabicFont();
  }, [locale]);

  const setLocale = useCallback(
    (next) => {
      if (!dictionaries[next] || !config.languages.includes(next)) return;
      setLocaleState(next);
      writeStorage(STORAGE_KEY, next);
    },
    [config]
  );

  // configureLanguages applies the shop's enabled languages + default (from
  // /v1/shop/settings) and re-resolves the active language against them.
  const configureLanguages = useCallback((raw) => {
    const next = normalizeConfig(raw);
    writeStorage(CONFIG_KEY, JSON.stringify(next));
    setConfig((prev) =>
      prev.defaultLanguage === next.defaultLanguage && prev.languages.join() === next.languages.join() ? prev : next
    );
    setLocaleState((current) => pickLocale(next, current));
  }, []);

  const value = useMemo(
    () => ({
      locale,
      setLocale,
      dir: dirFor(locale),
      languages: LANGUAGES.filter((l) => config.languages.includes(l.code)),
      defaultLanguage: config.defaultLanguage,
      configureLanguages,
      t: (key, vars) => translate(locale, key, vars),
      errMsg: (error) => errorMessage(error, locale),
      formatDate: (value, opts) => {
        if (!value) return "";
        const d = new Date(value);
        if (Number.isNaN(d.getTime())) return "";
        return d.toLocaleDateString(intlLocale(locale), opts || { year: "numeric", month: "short", day: "numeric" });
      },
      formatDateTime: (value) => {
        if (!value) return "";
        const d = new Date(value);
        if (Number.isNaN(d.getTime())) return "";
        return d.toLocaleString(intlLocale(locale), { dateStyle: "medium", timeStyle: "short" });
      },
    }),
    [locale, setLocale, config, configureLanguages]
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
};

export const useI18n = () => useContext(I18nContext);
