import { useState } from "react";
import { toast } from "react-toastify";
import { FiEdit2, FiPlus, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../../api/client";
import { useAsync } from "../../../lib/useAsync";
import { slugify, textToList } from "../../../lib/util";
import { useI18n } from "../../../i18n/I18nContext";
import { ConfirmModal, ErrorBox, Field, Modal, SelectField, Spinner, TextArea, TextField, Toggle } from "../../../components/ui";
import { useAllCountries } from "../useAllCountries";

const METHODS = ["flat_rate", "free_shipping", "local_pickup"];
const isYes = (v) => v === true || v === "yes";

const MethodModal = ({ zone, method, classes, onClose, onSaved }) => {
  const { t, errMsg } = useI18n();
  const [m, setM] = useState({ enabled: method.enabled !== false, order: method.order || 0, settings: { ...(method.settings || {}) } });
  const [busy, setBusy] = useState(false);
  const s = m.settings;
  const set = (k) => (v) => setM((x) => ({ ...x, settings: { ...x.settings, [k]: v } }));
  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      await api.put(`/v1/admin/shipping/zones/${zone.id}/methods/${method.instanceId}`, m);
      toast.success(t("admin.saved"));
      onSaved();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal show title={t("shipping.editMethod", { name: s.title || t(`shipping.methods.${method.methodId}`) })} onClose={onClose} wide>
      <form className="stack" onSubmit={save}>
        <Toggle checked={m.enabled} onChange={(enabled) => setM((x) => ({ ...x, enabled }))} label={t("shipping.enabled")} />
        <TextField label={t("shipping.title")} value={s.title} onChange={set("title")} />
        {(method.methodId === "flat_rate" || method.methodId === "local_pickup") && (
          <>
            <SelectField label={t("shipping.taxStatus")} value={s.tax_status || "taxable"} onChange={set("tax_status")} options={["taxable", "none"].map((x) => ({ value: x, label: t(`taxStatus.${x}`) }))} />
            <TextField label={t("shipping.cost")} hint={t("shipping.costHint")} value={s.cost} onChange={set("cost")} />
          </>
        )}
        {method.methodId === "flat_rate" && classes.length > 0 && (
          <fieldset className="fieldset stack">
            <legend>{t("shipping.classCosts")}</legend>
            {classes.map((c) => (
              <TextField key={c.id} label={t("shipping.classCost", { name: c.name })} value={s[`class_cost_${c.id}`]} onChange={set(`class_cost_${c.id}`)} />
            ))}
            <TextField label={t("shipping.noClassCost")} value={s.no_class_cost} onChange={set("no_class_cost")} />
            <SelectField label={t("shipping.calcType")} value={s.type || "class"} onChange={set("type")} options={["class", "order"].map((x) => ({ value: x, label: t(`shipping.calc.${x}`) }))} />
          </fieldset>
        )}
        {method.methodId === "free_shipping" && (
          <>
            <SelectField
              label={t("shipping.requires")}
              value={s.requires || ""}
              onChange={set("requires")}
              options={["", "coupon", "min_amount", "either", "both"].map((x) => ({ value: x, label: t(`shipping.req.${x || "none"}`) }))}
            />
            <TextField label={t("shipping.minAmount")} type="number" step="0.01" min="0" value={s.min_amount} onChange={set("min_amount")} />
            <Toggle checked={isYes(s.ignore_discounts)} onChange={(on) => set("ignore_discounts")(on ? "yes" : "no")} label={t("shipping.ignoreDiscounts")} />
          </>
        )}
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("common.save")}
          </button>
        </div>
      </form>
    </Modal>
  );
};

const ZoneModal = ({ zone, classes, onClose, onSaved }) => {
  const { t, errMsg } = useI18n();
  const countries = useAllCountries();
  const isRest = Number(zone.id) === 0 && !zone._new;
  const locs = zone.locations || [];
  const [name, setName] = useState(zone.name || "");
  const [regions, setRegions] = useState(locs.filter((l) => l.type !== "postcode").map((l) => `${l.type}:${l.code}`));
  const [postcodes, setPostcodes] = useState(locs.filter((l) => l.type === "postcode").map((l) => l.code).join("\n"));
  const [addMethod, setAddMethod] = useState("flat_rate");
  const [editingMethod, setEditingMethod] = useState(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [current, setCurrent] = useState(zone);

  const regionOptions = [];
  countries.forEach((c) => {
    regionOptions.push({ value: `country:${c.code}`, label: c.name });
    (c.states || []).forEach((s) => regionOptions.push({ value: `state:${c.code}:${s.code}`, label: `${c.name} — ${s.name}` }));
  });

  const refresh = async (id) => {
    const { data } = await api.get("/v1/admin/shipping/zones");
    const z = asArray(data).find((x) => Number(x.id) === Number(id));
    if (z) setCurrent(z);
    onSaved(false);
  };

  const save = async (e) => {
    e?.preventDefault?.();
    setBusy(true);
    setError(null);
    try {
      const locations = [
        ...regions.map((r) => {
          const [type, ...rest] = r.split(":");
          return { type, code: rest.join(":") };
        }),
        ...textToList(postcodes, /\n+/).map((code) => ({ type: "postcode", code })),
      ];
      const body = { name, order: current.order || 0, locations };
      let id = current.id;
      if (current._new) {
        const { data } = await api.post("/v1/admin/shipping/zones", body);
        id = data?.id;
        setCurrent({ ...(data || {}), _new: false });
      } else if (!isRest) {
        await api.put(`/v1/admin/shipping/zones/${current.id}`, body);
      }
      toast.success(t("admin.saved"));
      if (id !== undefined) await refresh(id);
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const method = async (fn) => {
    try {
      await fn();
      await refresh(current.id);
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <Modal show title={isRest ? t("shipping.restOfWorld") : current._new ? t("shipping.addZone") : t("shipping.editZone", { name: current.name })} onClose={() => onClose()} wide>
      <div className="stack">
        {!isRest && (
          <form className="stack" onSubmit={save}>
            <TextField label={t("shipping.zoneName")} value={name} onChange={setName} required />
            <Field label={t("shipping.regions")} hint={t("settings.multiHint")}>
              {(id) => (
                <select id={id} className="input" multiple size={8} value={regions} onChange={(e) => setRegions(Array.from(e.target.selectedOptions, (o) => o.value))}>
                  {regionOptions.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              )}
            </Field>
            <TextArea label={t("shipping.postcodes")} hint={t("shipping.postcodesHint")} value={postcodes} onChange={setPostcodes} rows={3} />
            <ErrorBox error={error} />
            <div>
              <button type="submit" className="btn btn-primary" disabled={busy}>
                {t("shipping.saveZone")}
              </button>
            </div>
          </form>
        )}
        {!current._new && (
          <section className="stack">
            <h3>{t("shipping.methodsTitle")}</h3>
            <ul className="plain-list">
              {(current.methods || []).map((m) => (
                <li key={m.instanceId} className="list-row">
                  <span>
                    <strong>{m.title || m.settings?.title || t(`shipping.methods.${m.methodId}`)}</strong> <span className="muted small">{t(`shipping.methods.${m.methodId}`)}</span>
                  </span>
                  <span className="row-actions">
                    <Toggle checked={m.enabled !== false} onChange={(enabled) => method(() => api.put(`/v1/admin/shipping/zones/${current.id}/methods/${m.instanceId}`, { ...m, enabled }))} label={<span className="sr-only">{t("shipping.enabled")}</span>} />
                    <button type="button" className="icon-btn" onClick={() => setEditingMethod(m)} aria-label={t("common.edit")}>
                      <FiEdit2 />
                    </button>
                    <button type="button" className="icon-btn danger" onClick={() => method(() => api.delete(`/v1/admin/shipping/zones/${current.id}/methods/${m.instanceId}`))} aria-label={t("common.delete")}>
                      <FiTrash2 />
                    </button>
                  </span>
                </li>
              ))}
              {(current.methods || []).length === 0 && <li className="muted small">{t("shipping.noMethods")}</li>}
            </ul>
            <div className="row">
              <label className="sr-only" htmlFor="add-method">
                {t("shipping.addMethod")}
              </label>
              <select id="add-method" className="input input-sm" value={addMethod} onChange={(e) => setAddMethod(e.target.value)}>
                {METHODS.map((x) => (
                  <option key={x} value={x}>
                    {t(`shipping.methods.${x}`)}
                  </option>
                ))}
              </select>
              <button type="button" className="btn btn-secondary btn-sm" onClick={() => method(() => api.post(`/v1/admin/shipping/zones/${current.id}/methods`, { methodId: addMethod }))}>
                <FiPlus aria-hidden="true" /> {t("shipping.addMethod")}
              </button>
            </div>
          </section>
        )}
      </div>
      {editingMethod && (
        <MethodModal
          zone={current}
          method={editingMethod}
          classes={classes}
          onClose={() => setEditingMethod(null)}
          onSaved={() => {
            setEditingMethod(null);
            refresh(current.id);
          }}
        />
      )}
    </Modal>
  );
};

const ShippingClasses = ({ classes, reload }) => {
  const { t, errMsg } = useI18n();
  const empty = { id: 0, name: "", slug: "", description: "" };
  const [form, setForm] = useState(empty);
  const save = async (e) => {
    e.preventDefault();
    try {
      const body = { ...form, slug: form.slug || slugify(form.name) };
      if (form.id) await api.put(`/v1/admin/shipping/classes/${form.id}`, body);
      else await api.post("/v1/admin/shipping/classes", body);
      setForm(empty);
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };
  return (
    <section className="card stack">
      <h2>{t("shipping.classes")}</h2>
      <p className="muted small">{t("shipping.classesHint")}</p>
      <ul className="plain-list">
        {classes.map((c) => (
          <li key={c.id} className="list-row">
            <span>
              <strong>{c.name}</strong> <span className="muted small">{c.slug}</span> {c.description && <span className="small">— {c.description}</span>}{" "}
              <span className="muted small">({t("shipping.productCount", { count: c.count ?? 0 })})</span>
            </span>
            <span className="row-actions">
              <button type="button" className="icon-btn" onClick={() => setForm({ ...empty, ...c })} aria-label={t("common.edit")}>
                <FiEdit2 />
              </button>
              <button
                type="button"
                className="icon-btn danger"
                aria-label={t("common.delete")}
                onClick={async () => {
                  try {
                    await api.delete(`/v1/admin/shipping/classes/${c.id}`);
                    reload();
                  } catch (err) {
                    toast.error(errMsg(err));
                  }
                }}
              >
                <FiTrash2 />
              </button>
            </span>
          </li>
        ))}
      </ul>
      <form className="form-grid-3" onSubmit={save}>
        <TextField label={t("admin.terms.name")} value={form.name} onChange={(name) => setForm({ ...form, name })} required />
        <TextField label={t("admin.terms.slug")} value={form.slug} onChange={(slug) => setForm({ ...form, slug: slugify(slug) })} placeholder={slugify(form.name)} />
        <TextField label={t("admin.terms.description")} value={form.description} onChange={(description) => setForm({ ...form, description })} />
        <div className="row">
          <button type="submit" className="btn btn-secondary btn-sm">
            {form.id ? t("common.update") : t("shipping.addClass")}
          </button>
          {form.id > 0 && (
            <button type="button" className="link-btn small" onClick={() => setForm(empty)}>
              {t("common.cancel")}
            </button>
          )}
        </div>
      </form>
    </section>
  );
};

const ShippingTab = () => {
  const { t, errMsg } = useI18n();
  const countries = useAllCountries();
  const { data: zones, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/shipping/zones")).data), [], []);
  const { data: classes, reload: reloadClasses } = useAsync(async () => asArray((await api.get("/v1/admin/shipping/classes")).data), [], []);
  const [editing, setEditing] = useState(null);
  const [deleting, setDeleting] = useState(null);

  const locLabel = (l) => {
    if (l.type === "country") return countries.find((c) => c.code === l.code)?.name || l.code;
    if (l.type === "state") {
      const [cc, st] = l.code.split(":");
      const c = countries.find((x) => x.code === cc);
      return `${c?.states?.find((s) => s.code === st)?.name || st}, ${c?.name || cc}`;
    }
    return l.code;
  };

  const sorted = [...zones].sort((a, b) => (Number(a.id) === 0 ? 1 : Number(b.id) === 0 ? -1 : (a.order || 0) - (b.order || 0)));

  return (
    <div className="stack-lg">
      <section className="card stack">
        <div className="section-head">
          <h2>{t("shipping.zones")}</h2>
          <button type="button" className="btn btn-primary btn-sm" onClick={() => setEditing({ id: 0, _new: true, name: "", locations: [], methods: [], order: zones.length })}>
            {t("shipping.addZone")}
          </button>
        </div>
        <p className="muted small">{t("shipping.zonesHint")}</p>
        <ErrorBox error={error} />
        {loading ? (
          <Spinner />
        ) : (
          <div className="table-wrap">
            <table className="table admin-table">
              <thead>
                <tr>
                  <th scope="col">{t("shipping.zoneName")}</th>
                  <th scope="col">{t("shipping.regions")}</th>
                  <th scope="col">{t("shipping.methodsTitle")}</th>
                  <th scope="col">
                    <span className="sr-only">{t("common.actions")}</span>
                  </th>
                </tr>
              </thead>
              <tbody>
                {sorted.map((z) => (
                  <tr key={z.id}>
                    <td>
                      <button type="button" className="link-btn" onClick={() => setEditing(z)}>
                        <strong>{Number(z.id) === 0 ? t("shipping.restOfWorld") : z.name}</strong>
                      </button>
                    </td>
                    <td className="small">{Number(z.id) === 0 ? t("shipping.restHint") : (z.locations || []).map(locLabel).join(", ") || "—"}</td>
                    <td className="small">
                      {(z.methods || []).map((m) => (
                        <span key={m.instanceId} className={`chip chip-sm ${m.enabled === false ? "is-off" : ""}`}>
                          {m.title || m.settings?.title || t(`shipping.methods.${m.methodId}`)}
                        </span>
                      ))}
                      {(z.methods || []).length === 0 && <span className="muted">{t("shipping.noMethods")}</span>}
                    </td>
                    <td className="row-actions">
                      <button type="button" className="icon-btn" onClick={() => setEditing(z)} aria-label={t("common.edit")}>
                        <FiEdit2 />
                      </button>
                      {Number(z.id) !== 0 && (
                        <button type="button" className="icon-btn danger" onClick={() => setDeleting(z)} aria-label={t("common.delete")}>
                          <FiTrash2 />
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
      <ShippingClasses classes={classes} reload={reloadClasses} />
      {editing && (
        <ZoneModal
          zone={editing}
          classes={classes}
          onClose={() => {
            setEditing(null);
            reload();
          }}
          onSaved={() => reload()}
        />
      )}
      <ConfirmModal
        show={!!deleting}
        title={t("shipping.deleteZone")}
        body={deleting ? t("shipping.deleteZoneBody", { name: deleting.name }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          const z = deleting;
          setDeleting(null);
          try {
            await api.delete(`/v1/admin/shipping/zones/${z.id}`);
            reload();
          } catch (err) {
            toast.error(errMsg(err));
          }
        }}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export default ShippingTab;
