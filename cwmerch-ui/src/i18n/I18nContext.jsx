import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import en from "./en.json";
import fr from "./fr.json";

export const dictionaries = { en, fr };
export const LANGUAGES = [
  { code: "en", label: "English" },
  { code: "fr", label: "Français" },
];
const STORAGE_KEY = "cwmerch.locale";

const resolve = (dict, key) =>
  key.split(".").reduce((acc, part) => (acc && acc[part] !== undefined ? acc[part] : undefined), dict);

const initialLocale = () => {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (stored && dictionaries[stored]) return stored;
  } catch {
    /* storage unavailable */
  }
  const browser = (typeof navigator !== "undefined" && (navigator.language || "en")).slice(0, 2).toLowerCase();
  return dictionaries[browser] ? browser : "en";
};

export const translate = (locale, key, vars) => {
  let str = resolve(dictionaries[locale], key) ?? resolve(dictionaries.en, key);
  if (typeof str !== "string") str = key;
  if (vars) {
    Object.keys(vars).forEach((k) => {
      str = str.split(`{{${k}}}`).join(vars[k] ?? "");
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

const I18nContext = createContext(null);

export const I18nProvider = ({ children }) => {
  const [locale, setLocaleState] = useState(initialLocale);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback((next) => {
    if (!dictionaries[next]) return;
    setLocaleState(next);
    try {
      localStorage.setItem(STORAGE_KEY, next);
    } catch {
      /* storage unavailable */
    }
  }, []);

  const value = useMemo(
    () => ({
      locale,
      setLocale,
      t: (key, vars) => translate(locale, key, vars),
      errMsg: (error) => errorMessage(error, locale),
      formatDate: (value, opts) => {
        if (!value) return "";
        const d = new Date(value);
        if (Number.isNaN(d.getTime())) return "";
        return d.toLocaleDateString(locale, opts || { year: "numeric", month: "short", day: "numeric" });
      },
      formatDateTime: (value) => {
        if (!value) return "";
        const d = new Date(value);
        if (Number.isNaN(d.getTime())) return "";
        return d.toLocaleString(locale, { dateStyle: "medium", timeStyle: "short" });
      },
    }),
    [locale, setLocale]
  );

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
};

export const useI18n = () => useContext(I18nContext);
