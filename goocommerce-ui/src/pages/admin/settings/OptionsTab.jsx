import { useEffect, useMemo, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../../api/client";
import { useShop } from "../../../context/ShopContext";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Field, Spinner, Toggle } from "../../../components/ui";
import { useAllCountries } from "../useAllCountries";

let cached = null;

// choicesOf accepts {value: label} or [{value, label}] or [value].
const choicesOf = (choices) => {
  if (!choices) return [];
  if (Array.isArray(choices)) return choices.map((c) => (typeof c === "object" ? { value: String(c.value ?? c.code ?? c.id), label: c.label ?? c.name ?? String(c.value) } : { value: String(c), label: String(c) }));
  return Object.entries(choices).map(([value, label]) => ({ value, label: String(label) }));
};

// A WooCommerce checkbox option is stored as "yes"/"no"; keep that wire type.
const isOn = (v) => v === true || v === "yes" || v === "1" || v === 1;
const toWire = (original, on) => (typeof original === "boolean" ? on : on ? "yes" : "no");

const OptionField = ({ field, value, onChange, countries }) => {
  const { t } = useI18n();
  const choices = choicesOf(field.choices);
  const countryChoices = useMemo(() => {
    if (choices.length) return choices;
    const out = [];
    countries.forEach((c) => {
      if (c.states?.length) c.states.forEach((s) => out.push({ value: `${c.code}:${s.code}`, label: `${c.name} — ${s.name}` }));
      else out.push({ value: c.code, label: c.name });
    });
    return out;
  }, [choices, countries]);

  switch (field.type) {
    case "checkbox":
      return <Toggle checked={isOn(value)} onChange={(on) => onChange(toWire(value, on))} label={field.label} hint={field.description} />;
    case "textarea":
      return (
        <Field label={field.label} hint={field.description}>
          {(id) => <textarea id={id} className="input" rows={4} value={value ?? ""} onChange={(e) => onChange(e.target.value)} />}
        </Field>
      );
    case "number":
      return (
        <Field label={field.label} hint={field.description}>
          {(id) => <input id={id} className="input" type="number" value={value ?? ""} onChange={(e) => onChange(e.target.value)} />}
        </Field>
      );
    case "select":
    case "country":
    case "single_select_country":
      return (
        <Field label={field.label} hint={field.description}>
          {(id) => (
            <select id={id} className="input" value={value ?? ""} onChange={(e) => onChange(e.target.value)}>
              <option value="">{t("settings.choose")}</option>
              {(field.type === "select" ? choices : countryChoices).map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          )}
        </Field>
      );
    case "multiselect":
    case "multi_select_countries": {
      const list = Array.isArray(value) ? value.map(String) : [];
      const opts = choices.length ? choices : countries.map((c) => ({ value: c.code, label: c.name }));
      return (
        <Field label={field.label} hint={field.description || t("settings.multiHint")}>
          {(id) => (
            <select id={id} className="input" multiple size={Math.min(8, Math.max(3, opts.length))} value={list} onChange={(e) => onChange(Array.from(e.target.selectedOptions, (o) => o.value))}>
              {opts.map((c) => (
                <option key={c.value} value={c.value}>
                  {c.label}
                </option>
              ))}
            </select>
          )}
        </Field>
      );
    }
    default:
      return (
        <Field label={field.label} hint={field.description}>
          {(id) => <input id={id} className="input" type="text" value={value ?? ""} onChange={(e) => onChange(e.target.value)} />}
        </Field>
      );
  }
};

// OptionsTab renders one settings group from GET /v1/admin/settings fields metadata.
const OptionsTab = ({ group }) => {
  const { t, errMsg } = useI18n();
  const { reload: reloadShop } = useShop();
  const countries = useAllCountries();
  const [data, setData] = useState(cached);
  const [draft, setDraft] = useState({});
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .get("/v1/admin/settings")
      .then(({ data: d }) => {
        cached = d;
        setData(d);
      })
      .catch(setError);
  }, []);

  useEffect(() => setDraft({}), [group]);

  if (!data) return error ? <ErrorBox error={error} /> : <Spinner />;
  const fields = (data.fields || []).filter((f) => (f.group || "general") === group);
  const value = (name) => (name in draft ? draft[name] : data.options?.[name]);
  const dirty = Object.keys(draft).length > 0;

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const { data: saved } = await api.put("/v1/admin/settings", { options: draft });
      const next = saved?.options ? { ...data, ...saved } : { ...data, options: { ...data.options, ...draft } };
      cached = next;
      setData(next);
      setDraft({});
      toast.success(t("admin.saved"));
      reloadShop();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  // Group consecutive fields under their optional `section` heading.
  const sections = [];
  fields.forEach((f) => {
    const name = f.section || "";
    const last = sections[sections.length - 1];
    if (last && last.name === name) last.fields.push(f);
    else sections.push({ name, fields: [f] });
  });

  return (
    <form className="stack-lg" onSubmit={save}>
      {fields.length === 0 && <p className="muted">{t("settings.noFields")}</p>}
      {sections.map((s, i) => (
        <section key={i} className="card stack settings-section">
          {s.name && <h2>{s.name}</h2>}
          {s.fields.map((f) => (
            <OptionField key={f.name} field={f} value={value(f.name)} countries={countries} onChange={(v) => setDraft((d) => ({ ...d, [f.name]: v }))} />
          ))}
        </section>
      ))}
      {fields.length > 0 && (
        <div className="sticky-save">
          <button type="submit" className="btn btn-primary" disabled={busy || !dirty}>
            {t("common.saveChanges")}
          </button>
        </div>
      )}
    </form>
  );
};

export default OptionsTab;
