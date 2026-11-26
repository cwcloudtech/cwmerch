import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiTrash2 } from "react-icons/fi";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { dateInput, dateToApi, idsToText, listToText, numOrNull, textToIds, textToList } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead, SearchBox } from "../../components/admin";
import { ConfirmModal, ErrorBox, Pagination, SelectField, Spinner, Tabs, TextArea, TextField, Toggle } from "../../components/ui";

const DISCOUNT_TYPES = ["percent", "fixed_cart", "fixed_product"];

export const Coupons = () => {
  const { t, formatDate } = useI18n();
  const { money } = useShop();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const { data, loading, error } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/coupons", { params: { page, perPage: 20, search: search || undefined } })).data, page),
    [page, search],
    { items: [], totalPages: 1 }
  );
  return (
    <div className="stack">
      <PageHead title={t("admin.nav.coupons")}>
        <Link to="/admin/coupons/new" className="btn btn-primary btn-sm">
          {t("admin.coupons.add")}
        </Link>
      </PageHead>
      <div className="toolbar">
        <span />
        <SearchBox
          value={search}
          onSearch={(s) => {
            setSearch(s);
            setPage(1);
          }}
        />
      </div>
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : (
        <div className="table-wrap">
          <table className="table admin-table">
            <thead>
              <tr>
                <th scope="col">{t("admin.coupons.code")}</th>
                <th scope="col">{t("admin.coupons.type")}</th>
                <th scope="col" className="num">
                  {t("admin.coupons.amount")}
                </th>
                <th scope="col">{t("admin.coupons.description")}</th>
                <th scope="col">{t("admin.coupons.usage")}</th>
                <th scope="col">{t("admin.coupons.expiry")}</th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((c) => (
                <tr key={c.id}>
                  <td>
                    <Link to={`/admin/coupons/${c.id}`}>
                      <code>{c.code}</code>
                    </Link>
                    {c.status && c.status !== "publish" && <span className="muted small"> — {t(`productStatus.${c.status}`)}</span>}
                  </td>
                  <td>{t(`admin.coupons.types.${c.discountType}`)}</td>
                  <td className="num">{c.discountType === "percent" ? `${c.amount} %` : money(c.amount)}</td>
                  <td className="small">{c.description}</td>
                  <td>
                    {c.usageCount || 0} / {c.usageLimit || "∞"}
                  </td>
                  <td>{c.dateExpires ? formatDate(c.dateExpires) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {data.items.length === 0 && <p className="muted">{t("admin.coupons.none")}</p>}
        </div>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
    </div>
  );
};

const NEW_COUPON = {
  id: 0,
  code: "",
  amount: "",
  discountType: "percent",
  description: "",
  dateExpires: null,
  individualUse: false,
  productIds: [],
  excludedProductIds: [],
  usageLimit: null,
  usageLimitPerUser: null,
  limitUsageToXItems: null,
  freeShipping: false,
  productCategories: [],
  excludedProductCategories: [],
  excludeSaleItems: false,
  minimumAmount: "",
  maximumAmount: "",
  emailRestrictions: [],
  status: "publish",
};

const CategoryMulti = ({ label, value, onChange, categories }) => (
  <fieldset className="fieldset">
    <legend>{label}</legend>
    <div className="chip-row">
      {categories.map((c) => {
        const on = (value || []).map(Number).includes(Number(c.id));
        return (
          <label key={c.id} className={`chip ${on ? "is-active" : ""}`}>
            <input type="checkbox" className="sr-only" checked={on} onChange={() => onChange(on ? value.filter((x) => Number(x) !== Number(c.id)) : [...(value || []), c.id])} />
            {c.name}
          </label>
        );
      })}
    </div>
  </fieldset>
);

// TextListField keeps the raw text while typing and parses it on blur.
const TextListField = ({ label, hint, value, onChange, parse, format }) => {
  const [text, setText] = useState(format(value));
  useEffect(() => setText(format(value)), [value, format]);
  return <TextField label={label} hint={hint} value={text} onChange={setText} onBlur={() => onChange(parse(text))} />;
};

export const CouponEditor = () => {
  const { id } = useParams();
  const isNew = !id || id === "new";
  const navigate = useNavigate();
  const { t, errMsg } = useI18n();
  const { categories } = useShop();
  const [c, setC] = useState(isNew ? NEW_COUPON : null);
  const [tab, setTab] = useState("general");
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (isNew) return;
    api
      .get(`/v1/admin/coupons/${id}`)
      .then(({ data }) => setC({ ...NEW_COUPON, ...data }))
      .catch(setError);
  }, [id, isNew]);

  if (!c) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setC((x) => ({ ...x, [k]: v }));

  const generate = () => {
    const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789";
    const bytes = new Uint8Array(8);
    window.crypto.getRandomValues(bytes);
    set("code")(Array.from(bytes, (b) => chars[b % chars.length]).join(""));
  };

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body = {
        ...c,
        code: c.code.trim().toLowerCase(),
        amount: Number(c.amount || 0),
        usageLimit: numOrNull(c.usageLimit),
        usageLimitPerUser: numOrNull(c.usageLimitPerUser),
        limitUsageToXItems: numOrNull(c.limitUsageToXItems),
        minimumAmount: c.minimumAmount === "" ? null : Number(c.minimumAmount),
        maximumAmount: c.maximumAmount === "" ? null : Number(c.maximumAmount),
      };
      const { data } = isNew ? await api.post("/v1/admin/coupons", body) : await api.put(`/v1/admin/coupons/${c.id}`, body);
      toast.success(t("admin.saved"));
      if (data?.id) {
        setC({ ...NEW_COUPON, ...data });
        if (isNew) navigate(`/admin/coupons/${data.id}`, { replace: true });
      }
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="stack-lg">
      <PageHead title={isNew ? t("admin.coupons.add") : t("admin.coupons.edit", { code: c.code })}>
        <Link to="/admin/coupons" className="btn btn-secondary btn-sm">
          {t("common.back")}
        </Link>
      </PageHead>
      <ErrorBox error={error} />
      <form className="admin-columns" onSubmit={save}>
        <div className="stack-lg">
          <section className="card stack">
            <div className="row wrap align-end">
              <TextField label={t("admin.coupons.code")} value={c.code} onChange={set("code")} required className="grow" />
              <button type="button" className="btn btn-secondary btn-sm" onClick={generate}>
                {t("admin.coupons.generate")}
              </button>
            </div>
            <TextArea label={t("admin.coupons.description")} value={c.description} onChange={set("description")} rows={2} />
          </section>
          <section className="card stack">
            <h2>{t("admin.coupons.data")}</h2>
            <Tabs
              tabs={[
                { id: "general", label: t("admin.coupons.tabGeneral") },
                { id: "restrictions", label: t("admin.coupons.tabRestrictions") },
                { id: "limits", label: t("admin.coupons.tabLimits") },
              ]}
              active={tab}
              onChange={setTab}
            />
            <div className="tab-panel" role="tabpanel">
              {tab === "general" && (
                <div className="form-grid">
                  <SelectField label={t("admin.coupons.type")} value={c.discountType} onChange={set("discountType")} options={DISCOUNT_TYPES.map((x) => ({ value: x, label: t(`admin.coupons.types.${x}`) }))} />
                  <TextField label={t("admin.coupons.amount")} type="number" step="0.01" min="0" value={c.amount} onChange={set("amount")} required />
                  <TextField label={t("admin.coupons.expiry")} type="date" value={dateInput(c.dateExpires)} onChange={(d) => set("dateExpires")(dateToApi(d))} />
                  <div className="span-2">
                    <Toggle checked={!!c.freeShipping} onChange={set("freeShipping")} label={t("admin.coupons.freeShipping")} />
                  </div>
                </div>
              )}
              {tab === "restrictions" && (
                <div className="stack">
                  <div className="form-grid">
                    <TextField label={t("admin.coupons.minimum")} type="number" step="0.01" min="0" value={c.minimumAmount ?? ""} onChange={set("minimumAmount")} />
                    <TextField label={t("admin.coupons.maximum")} type="number" step="0.01" min="0" value={c.maximumAmount ?? ""} onChange={set("maximumAmount")} />
                  </div>
                  <Toggle checked={!!c.individualUse} onChange={set("individualUse")} label={t("admin.coupons.individualUse")} />
                  <Toggle checked={!!c.excludeSaleItems} onChange={set("excludeSaleItems")} label={t("admin.coupons.excludeSale")} />
                  <TextListField label={t("admin.coupons.products")} hint={t("admin.products.idsHint")} value={c.productIds} onChange={set("productIds")} parse={textToIds} format={idsToText} />
                  <TextListField label={t("admin.coupons.excludeProducts")} hint={t("admin.products.idsHint")} value={c.excludedProductIds} onChange={set("excludedProductIds")} parse={textToIds} format={idsToText} />
                  <CategoryMulti label={t("admin.coupons.categories")} value={c.productCategories} onChange={set("productCategories")} categories={categories} />
                  <CategoryMulti label={t("admin.coupons.excludeCategories")} value={c.excludedProductCategories} onChange={set("excludedProductCategories")} categories={categories} />
                  <TextListField label={t("admin.coupons.emails")} hint={t("admin.coupons.emailsHint")} value={c.emailRestrictions} onChange={set("emailRestrictions")} parse={(x) => textToList(x)} format={listToText} />
                </div>
              )}
              {tab === "limits" && (
                <div className="form-grid">
                  <TextField label={t("admin.coupons.usageLimit")} hint={t("admin.coupons.unlimited")} type="number" min="0" value={c.usageLimit ?? ""} onChange={set("usageLimit")} />
                  <TextField label={t("admin.coupons.limitItems")} hint={t("admin.coupons.limitItemsHint")} type="number" min="0" value={c.limitUsageToXItems ?? ""} onChange={set("limitUsageToXItems")} />
                  <TextField label={t("admin.coupons.perUser")} hint={t("admin.coupons.unlimited")} type="number" min="0" value={c.usageLimitPerUser ?? ""} onChange={set("usageLimitPerUser")} />
                </div>
              )}
            </div>
          </section>
        </div>
        <aside className="card stack">
          <h2>{t("admin.products.publish")}</h2>
          <SelectField label={t("admin.products.status")} value={c.status || "publish"} onChange={set("status")} options={["publish", "draft", "pending"].map((x) => ({ value: x, label: t(`productStatus.${x}`) }))} />
          {!isNew && <p className="small muted">{t("admin.coupons.usedTimes", { count: c.usageCount || 0 })}</p>}
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {isNew ? t("admin.coupons.publish") : t("common.update")}
          </button>
          {!isNew && (
            <button type="button" className="link-btn danger" onClick={() => setDeleting(true)}>
              <FiTrash2 aria-hidden="true" /> {t("common.delete")}
            </button>
          )}
        </aside>
      </form>
      <ConfirmModal
        show={deleting}
        title={t("common.delete")}
        body={t("admin.coupons.deleteBody", { code: c.code })}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          setDeleting(false);
          try {
            await api.delete(`/v1/admin/coupons/${c.id}`, { params: { force: true } });
            toast.success(t("admin.deleted"));
            navigate("/admin/coupons");
          } catch (err) {
            toast.error(errMsg(err));
          }
        }}
        onCancel={() => setDeleting(false)}
      />
    </div>
  );
};
