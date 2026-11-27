import { useState } from "react";
import { FiArrowDown, FiArrowUp, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { slugify, textToList } from "../../lib/util";
import { useI18n } from "../../i18n/I18nContext";
import { Toggle } from "../../components/ui";

// GlobalTerms lazily loads the terms of a global (pa_*) attribute and lets the
// editor tick which ones the product uses.
const GlobalTerms = ({ attrId, options, onChange }) => {
  const { t } = useI18n();
  const { data: terms } = useAsync(async () => asArray((await api.get(`/v1/admin/attributes/${attrId}/terms`)).data), [attrId], []);
  const toggle = (name) => onChange(options.includes(name) ? options.filter((o) => o !== name) : [...options, name]);
  return (
    <fieldset className="term-picker">
      <legend className="field-label">{t("admin.attributes.terms")}</legend>
      <div className="chip-row">
        {terms.map((term) => (
          <label key={term.id} className={`chip ${options.includes(term.name) ? "is-active" : ""}`}>
            <input type="checkbox" className="sr-only" checked={options.includes(term.name)} onChange={() => toggle(term.name)} />
            {term.name}
          </label>
        ))}
        {terms.length === 0 && <span className="muted small">{t("admin.attributes.noTerms")}</span>}
      </div>
      <div className="row">
        <button type="button" className="link-btn small" onClick={() => onChange(terms.map((x) => x.name))}>
          {t("admin.attributes.selectAll")}
        </button>
        <button type="button" className="link-btn small" onClick={() => onChange([])}>
          {t("admin.attributes.selectNone")}
        </button>
      </div>
    </fieldset>
  );
};

const ProductAttributes = ({ attributes, onChange, isVariable }) => {
  const { t } = useI18n();
  const { data: globals } = useAsync(async () => asArray((await api.get("/v1/admin/attributes")).data), [], []);
  const [pick, setPick] = useState("");
  const list = attributes || [];
  const set = (i, patch) => onChange(list.map((a, j) => (j === i ? { ...a, ...patch } : a)));
  const move = (i, d) => {
    const j = i + d;
    if (j < 0 || j >= list.length) return;
    const next = [...list];
    [next[i], next[j]] = [next[j], next[i]];
    onChange(next.map((a, k) => ({ ...a, position: k })));
  };

  const add = () => {
    if (pick === "custom") {
      onChange([...list, { id: 0, name: "", slug: "", position: list.length, visible: true, variation: isVariable, options: [] }]);
    } else {
      const g = globals.find((x) => String(x.id) === pick);
      if (!g || list.some((a) => a.slug === g.slug)) return;
      onChange([...list, { id: g.id, name: g.name, slug: g.slug, position: list.length, visible: true, variation: isVariable, options: [] }]);
    }
    setPick("");
  };

  return (
    <div className="stack">
      <div className="row">
        <label className="sr-only" htmlFor="attr-pick">
          {t("admin.attributes.add")}
        </label>
        <select id="attr-pick" className="input input-sm" value={pick} onChange={(e) => setPick(e.target.value)}>
          <option value="">{t("admin.attributes.choose")}</option>
          <option value="custom">{t("admin.attributes.custom")}</option>
          {globals.map((g) => (
            <option key={g.id} value={g.id} disabled={list.some((a) => a.slug === g.slug)}>
              {g.name}
            </option>
          ))}
        </select>
        <button type="button" className="btn btn-secondary btn-sm" onClick={add} disabled={!pick}>
          {t("common.add")}
        </button>
      </div>
      {list.length === 0 && <p className="muted small">{t("admin.attributes.none")}</p>}
      {list.map((a, i) => {
        const global = Number(a.id) > 0;
        return (
          <div key={`${a.slug || "custom"}-${i}`} className="attr-row card-inner">
            <div className="section-head">
              <strong>{a.name || t("admin.attributes.custom")}</strong>
              <span className="row-actions">
                <button type="button" className="icon-btn" onClick={() => move(i, -1)} disabled={i === 0} aria-label={t("common.moveUp")}>
                  <FiArrowUp />
                </button>
                <button type="button" className="icon-btn" onClick={() => move(i, 1)} disabled={i === list.length - 1} aria-label={t("common.moveDown")}>
                  <FiArrowDown />
                </button>
                <button type="button" className="icon-btn danger" onClick={() => onChange(list.filter((_, j) => j !== i))} aria-label={t("common.remove")}>
                  <FiTrash2 />
                </button>
              </span>
            </div>
            {global ? (
              <GlobalTerms attrId={a.id} options={a.options || []} onChange={(options) => set(i, { options })} />
            ) : (
              <div className="form-grid">
                <div className="field">
                  <label className="field-label" htmlFor={`attr-name-${i}`}>
                    {t("admin.attributes.name")}
                  </label>
                  <input id={`attr-name-${i}`} className="input" value={a.name} onChange={(e) => set(i, { name: e.target.value, slug: slugify(e.target.value) })} />
                </div>
                <div className="field">
                  <label className="field-label" htmlFor={`attr-values-${i}`}>
                    {t("admin.attributes.values")}
                  </label>
                  <input
                    id={`attr-values-${i}`}
                    className="input"
                    defaultValue={(a.options || []).join(" | ")}
                    onBlur={(e) => set(i, { options: textToList(e.target.value, /\|/) })}
                    placeholder="S | M | L"
                  />
                  <p className="hint">{t("admin.attributes.valuesHint")}</p>
                </div>
              </div>
            )}
            <div className="row wrap">
              <Toggle checked={a.visible !== false} onChange={(visible) => set(i, { visible })} label={t("admin.attributes.visible")} />
              {isVariable && <Toggle checked={!!a.variation} onChange={(variation) => set(i, { variation })} label={t("admin.attributes.usedForVariations")} />}
            </div>
          </div>
        );
      })}
    </div>
  );
};

export default ProductAttributes;
