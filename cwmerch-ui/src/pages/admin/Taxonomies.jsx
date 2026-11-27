import { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiEdit2, FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { slugify } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, uploadMedia, UploadButton } from "../../components/admin";
import { ConfirmModal, ErrorBox, SelectField, Spinner, TextArea, TextField, Toggle } from "../../components/ui";

// TermManager is the WooCommerce "add on the left, table on the right" screen
// shared by categories, tags and attribute terms.
const TermManager = ({ title, endpoint, withParent, withImage, extraColumns = [], back, onChanged }) => {
  const { t, errMsg } = useI18n();
  const { data: items, loading, error, reload } = useAsync(async () => asArray((await api.get(endpoint, { params: { perPage: 100 } })).data), [endpoint], []);
  const empty = { id: 0, name: "", slug: "", description: "", parent: 0, image: null, menuOrder: 0 };
  const [form, setForm] = useState(empty);
  const [busy, setBusy] = useState(false);
  const [formError, setFormError] = useState(null);
  const [deleting, setDeleting] = useState(null);
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setFormError(null);
    try {
      const body = { ...form, slug: form.slug || slugify(form.name), parent: Number(form.parent || 0) };
      if (form.id) await api.put(`${endpoint}/${form.id}`, body);
      else await api.post(endpoint, body);
      toast.success(t("admin.saved"));
      setForm(empty);
      reload();
      onChanged?.();
    } catch (err) {
      setFormError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    const item = deleting;
    setDeleting(null);
    try {
      await api.delete(`${endpoint}/${item.id}`);
      toast.success(t("admin.deleted"));
      reload();
      onChanged?.();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  // Tree order with depth for categories.
  const ordered = [];
  const walk = (parent, depth) =>
    items
      .filter((c) => Number(c.parent || 0) === parent)
      .forEach((c) => {
        ordered.push({ ...c, depth });
        walk(Number(c.id), depth + 1);
      });
  if (withParent) {
    walk(0, 0);
    items.forEach((c) => !ordered.some((o) => o.id === c.id) && ordered.push({ ...c, depth: 0 }));
  }
  const rows = withParent ? ordered : items.map((c) => ({ ...c, depth: 0 }));

  return (
    <div className="stack-lg">
      <PageHead title={title}>{back}</PageHead>
      <div className="admin-columns reverse">
        <section className="card stack">
          <h2>{form.id ? t("admin.terms.edit") : t("admin.terms.add")}</h2>
          <form className="stack" onSubmit={submit}>
            <TextField label={t("admin.terms.name")} value={form.name} onChange={set("name")} required />
            <TextField label={t("admin.terms.slug")} value={form.slug} onChange={(v) => set("slug")(slugify(v))} placeholder={slugify(form.name)} />
            {withParent && (
              <SelectField
                label={t("admin.terms.parent")}
                value={form.parent || 0}
                onChange={set("parent")}
                options={[{ value: 0, label: t("admin.terms.none") }, ...rows.filter((c) => c.id !== form.id).map((c) => ({ value: c.id, label: `${"— ".repeat(c.depth)}${c.name}` }))]}
              />
            )}
            <TextArea label={t("admin.terms.description")} value={form.description} onChange={set("description")} rows={3} />
            {withImage && (
              <div className="field">
                <span className="field-label">{t("admin.terms.image")}</span>
                <div className="row">
                  {form.image?.src ? <img src={form.image.src} alt="" width="56" height="56" className="thumb-md" /> : <span className="img-placeholder thumb-md" />}
                  <UploadButton
                    label={t("admin.products.uploadImage")}
                    onFiles={async ([f]) => {
                      try {
                        const m = await uploadMedia(api, f);
                        set("image")({ id: m.id, src: m.src });
                      } catch (err) {
                        toast.error(errMsg(err));
                      }
                    }}
                  />
                  {form.image && (
                    <button type="button" className="link-btn small" onClick={() => set("image")(null)}>
                      {t("common.remove")}
                    </button>
                  )}
                </div>
              </div>
            )}
            <ErrorBox error={formError} />
            <div className="row">
              <button type="submit" className="btn btn-primary" disabled={busy}>
                {form.id ? t("common.update") : t("admin.terms.add")}
              </button>
              {form.id > 0 && (
                <button type="button" className="btn btn-secondary" onClick={() => setForm(empty)}>
                  {t("common.cancel")}
                </button>
              )}
            </div>
          </form>
        </section>
        <section>
          <ErrorBox error={error} />
          {loading ? (
            <Spinner />
          ) : (
            <div className="table-wrap">
              <table className="table admin-table">
                <thead>
                  <tr>
                    {withImage && (
                      <th scope="col">
                        <span className="sr-only">{t("admin.terms.image")}</span>
                      </th>
                    )}
                    <th scope="col">{t("admin.terms.name")}</th>
                    <th scope="col">{t("admin.terms.slug")}</th>
                    {extraColumns.map((c) => (
                      <th key={c.key} scope="col">
                        {c.label}
                      </th>
                    ))}
                    <th scope="col" className="num">
                      {t("admin.terms.count")}
                    </th>
                    <th scope="col">
                      <span className="sr-only">{t("common.actions")}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((c) => (
                    <tr key={c.id}>
                      {withImage && <td>{c.image?.src ? <img src={c.image.src} alt="" width="36" height="36" className="thumb-sm" /> : <span className="img-placeholder thumb-sm" />}</td>}
                      <td>
                        {"— ".repeat(c.depth)}
                        <strong>{c.name}</strong>
                      </td>
                      <td className="small">{c.slug}</td>
                      {extraColumns.map((col) => (
                        <td key={col.key}>{col.render(c)}</td>
                      ))}
                      <td className="num">{c.count ?? 0}</td>
                      <td className="row-actions">
                        <button type="button" className="icon-btn" onClick={() => setForm({ ...empty, ...c })} aria-label={t("common.edit")}>
                          <FiEdit2 />
                        </button>
                        <button type="button" className="icon-btn danger" onClick={() => setDeleting(c)} aria-label={t("common.delete")}>
                          <FiTrash2 />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {rows.length === 0 && <p className="muted">{t("admin.terms.empty")}</p>}
            </div>
          )}
        </section>
      </div>
      <ConfirmModal
        show={!!deleting}
        title={t("admin.terms.deleteTitle")}
        body={deleting ? t("admin.terms.deleteBody", { name: deleting.name }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={remove}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
};

export const Categories = () => {
  const { t } = useI18n();
  const { reload } = useShop();
  return <TermManager title={t("admin.nav.categories")} endpoint="/v1/admin/categories" withParent withImage onChanged={reload} />;
};

export const Tags = () => {
  const { t } = useI18n();
  return <TermManager title={t("admin.nav.tags")} endpoint="/v1/admin/tags" />;
};

export const AttributeTerms = () => {
  const { id } = useParams();
  const { t } = useI18n();
  const { data: attr } = useAsync(async () => asArray((await api.get("/v1/admin/attributes")).data).find((a) => String(a.id) === String(id)), [id]);
  return (
    <TermManager
      title={t("admin.attributes.termsOf", { name: attr?.name || `#${id}` })}
      endpoint={`/v1/admin/attributes/${id}/terms`}
      back={
        <Link to="/admin/attributes" className="btn btn-secondary btn-sm">
          {t("common.back")}
        </Link>
      }
    />
  );
};

export const Attributes = () => {
  const { t, errMsg } = useI18n();
  const { data: items, loading, error, reload } = useAsync(async () => asArray((await api.get("/v1/admin/attributes")).data), [], []);
  const empty = { id: 0, name: "", slug: "", type: "select", orderBy: "menu_order", hasArchives: false };
  const [form, setForm] = useState(empty);
  const [deleting, setDeleting] = useState(null);
  const [formError, setFormError] = useState(null);
  const set = (k) => (v) => setForm((f) => ({ ...f, [k]: v }));

  const submit = async (e) => {
    e.preventDefault();
    setFormError(null);
    try {
      const body = { ...form, slug: form.slug || slugify(form.name) };
      if (form.id) await api.put(`/v1/admin/attributes/${form.id}`, body);
      else await api.post("/v1/admin/attributes", body);
      toast.success(t("admin.saved"));
      setForm(empty);
      reload();
    } catch (err) {
      setFormError(errMsg(err));
    }
  };

  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.attributes")} />
      <div className="admin-columns reverse">
        <section className="card">
          <form className="stack" onSubmit={submit}>
            <h2>{form.id ? t("admin.attributes.edit") : t("admin.attributes.addNew")}</h2>
            <TextField label={t("admin.terms.name")} value={form.name} onChange={set("name")} required />
            <TextField label={t("admin.terms.slug")} hint={t("admin.attributes.slugHint")} value={form.slug} onChange={(v) => set("slug")(slugify(v).slice(0, 28))} placeholder={slugify(form.name)} />
            <Toggle checked={!!form.hasArchives} onChange={set("hasArchives")} label={t("admin.attributes.archives")} />
            <SelectField label={t("admin.attributes.type")} value={form.type} onChange={set("type")} options={["select", "text"].map((x) => ({ value: x, label: t(`admin.attributes.typeOpt.${x}`) }))} />
            <SelectField
              label={t("admin.attributes.orderBy")}
              value={form.orderBy}
              onChange={set("orderBy")}
              options={["menu_order", "name", "name_num", "id"].map((x) => ({ value: x, label: t(`admin.attributes.order.${x}`) }))}
            />
            <ErrorBox error={formError} />
            <div className="row">
              <button type="submit" className="btn btn-primary">
                {form.id ? t("common.update") : t("admin.attributes.addNew")}
              </button>
              {form.id > 0 && (
                <button type="button" className="btn btn-secondary" onClick={() => setForm(empty)}>
                  {t("common.cancel")}
                </button>
              )}
            </div>
          </form>
        </section>
        <section>
          <ErrorBox error={error} />
          {loading ? (
            <Spinner />
          ) : (
            <div className="table-wrap">
              <table className="table admin-table">
                <thead>
                  <tr>
                    <th scope="col">{t("admin.terms.name")}</th>
                    <th scope="col">{t("admin.terms.slug")}</th>
                    <th scope="col">{t("admin.attributes.orderBy")}</th>
                    <th scope="col">{t("admin.attributes.terms")}</th>
                    <th scope="col">
                      <span className="sr-only">{t("common.actions")}</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {items.map((a) => (
                    <tr key={a.id}>
                      <td>
                        <strong>{a.name}</strong>
                      </td>
                      <td className="small">{a.slug}</td>
                      <td className="small">{t(`admin.attributes.order.${a.orderBy || "menu_order"}`)}</td>
                      <td>
                        <Link to={`/admin/attributes/${a.id}/terms`} className="btn btn-secondary btn-sm">
                          {t("admin.attributes.configureTerms")}
                        </Link>
                      </td>
                      <td className="row-actions">
                        <button type="button" className="icon-btn" onClick={() => setForm({ ...empty, ...a })} aria-label={t("common.edit")}>
                          <FiEdit2 />
                        </button>
                        <button type="button" className="icon-btn danger" onClick={() => setDeleting(a)} aria-label={t("common.delete")}>
                          <FiTrash2 />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {items.length === 0 && <p className="muted">{t("admin.attributes.none")}</p>}
            </div>
          )}
        </section>
      </div>
      <ConfirmModal
        show={!!deleting}
        title={t("admin.terms.deleteTitle")}
        body={deleting ? t("admin.attributes.deleteBody", { name: deleting.name }) : ""}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          const a = deleting;
          setDeleting(null);
          try {
            await api.delete(`/v1/admin/attributes/${a.id}`);
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
