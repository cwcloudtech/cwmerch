import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { toast } from "react-toastify";
import { FiTrash2 } from "react-icons/fi";
import { api, asArray } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { ORDER_STATUSES } from "../../lib/util";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import AddressForm, { AddressView } from "../../components/AddressForm";
import { OrderTotals } from "../../components/OrderDetails";
import { PageHead } from "../../components/admin";
import { useAllCountries } from "./useAllCountries";
import { ConfirmModal, ErrorBox, Modal, Spinner, StatusBadge, TextArea, TextField, Toggle } from "../../components/ui";

const EMAIL_TYPES = ["customer_invoice", "new_order", "customer_processing_order", "customer_completed_order"];

const Notes = ({ orderId }) => {
  const { t, errMsg, formatDateTime } = useI18n();
  const { data: notes, reload } = useAsync(async () => asArray((await api.get(`/v1/admin/orders/${orderId}/notes`)).data), [orderId], []);
  const [note, setNote] = useState("");
  const [customerNote, setCustomerNote] = useState(false);
  const add = async (e) => {
    e.preventDefault();
    try {
      await api.post(`/v1/admin/orders/${orderId}/notes`, { note, customerNote });
      setNote("");
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };
  const remove = async (id) => {
    try {
      await api.delete(`/v1/admin/orders/${orderId}/notes/${id}`);
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };
  return (
    <section className="card stack">
      <h2>{t("admin.orders.notes")}</h2>
      <ul className="order-notes">
        {notes.map((n) => (
          <li key={n.id} className={n.customerNote ? "is-customer" : n.systemNote || !n.author ? "is-system" : ""}>
            <div className="note-body pre-line">{n.note}</div>
            <div className="note-meta small muted">
              {formatDateTime(n.createdAt)} {n.author && `· ${n.author}`} {n.customerNote && `· ${t("admin.orders.toCustomer")}`}
              <button type="button" className="icon-btn danger" onClick={() => remove(n.id)} aria-label={t("common.delete")}>
                <FiTrash2 />
              </button>
            </div>
          </li>
        ))}
        {notes.length === 0 && <li className="muted small">{t("admin.orders.noNotes")}</li>}
      </ul>
      <form className="stack" onSubmit={add}>
        <TextArea label={t("admin.orders.addNote")} value={note} onChange={setNote} rows={3} required />
        <Toggle checked={customerNote} onChange={setCustomerNote} label={t("admin.orders.noteToCustomer")} />
        <div>
          <button type="submit" className="btn btn-secondary btn-sm" disabled={!note.trim()}>
            {t("common.add")}
          </button>
        </div>
      </form>
    </section>
  );
};

const RefundModal = ({ order, show, onClose, onDone }) => {
  const { t, errMsg } = useI18n();
  const { money } = useShop();
  const [lines, setLines] = useState({});
  const [amount, setAmount] = useState("");
  const [reason, setReason] = useState("");
  const [restock, setRestock] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    if (show) {
      setLines({});
      setAmount("");
      setReason("");
      setError(null);
    }
  }, [show]);

  const unit = (l) => (Number(l.total || 0) + Number(l.totalTax || 0)) / (Number(l.quantity) || 1);
  const setLineQty = (l, q) => {
    const next = { ...lines, [l.id]: Math.max(0, Math.min(Number(l.quantity), Number(q) || 0)) };
    setLines(next);
    const sum = (order.lineItems || []).reduce((s, x) => s + (next[x.id] || 0) * unit(x), 0);
    setAmount(sum ? sum.toFixed(2) : "");
  };
  const max = Number(order.total || 0) - Number(order.totalRefunded || 0);

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post(`/v1/admin/orders/${order.id}/refunds`, {
        amount: Number(amount),
        reason,
        restock,
        lineItems: (order.lineItems || [])
          .filter((l) => lines[l.id] > 0)
          .map((l) => ({ id: l.id, quantity: lines[l.id], refundTotal: Number((lines[l.id] * unit(l)).toFixed(2)) })),
      });
      toast.success(t("admin.orders.refunded"));
      onDone();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal show={show} title={t("admin.orders.refund")} onClose={onClose} wide>
      <form className="stack" onSubmit={submit}>
        <table className="table">
          <thead>
            <tr>
              <th scope="col">{t("cart.product")}</th>
              <th scope="col" className="num">
                {t("cart.price")}
              </th>
              <th scope="col">{t("admin.orders.refundQty")}</th>
            </tr>
          </thead>
          <tbody>
            {(order.lineItems || []).map((l) => (
              <tr key={l.id}>
                <td>
                  {l.name} × {l.quantity}
                </td>
                <td className="num">{money(unit(l))}</td>
                <td>
                  <input
                    className="input qty-input"
                    type="number"
                    min="0"
                    max={l.quantity}
                    value={lines[l.id] || 0}
                    onChange={(e) => setLineQty(l, e.target.value)}
                    aria-label={t("product.quantityFor", { name: l.name })}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        <Toggle checked={restock} onChange={setRestock} label={t("admin.orders.restock")} />
        <div className="form-grid">
          <TextField label={t("admin.orders.refundAmount")} hint={t("admin.orders.refundMax", { max: money(max) })} type="number" step="0.01" min="0.01" max={max} value={amount} onChange={setAmount} required />
          <TextField label={t("admin.orders.refundReason")} value={reason} onChange={setReason} />
        </div>
        <ErrorBox error={error} />
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="submit" className="btn btn-danger" disabled={busy || !Number(amount)}>
            {t("admin.orders.refundButton", { amount: money(Number(amount) || 0) })}
          </button>
        </div>
      </form>
    </Modal>
  );
};

const OrderDetail = () => {
  const { id } = useParams();
  const { t, errMsg, formatDateTime } = useI18n();
  const { money } = useShop();
  const countries = useAllCountries();
  const { data: order, loading, error, reload, setData } = useAsync(async () => (await api.get(`/v1/admin/orders/${id}`)).data, [id]);
  const [status, setStatus] = useState("");
  const [editing, setEditing] = useState(null);
  const [draft, setDraft] = useState(null);
  const [refundOpen, setRefundOpen] = useState(false);
  const [emailType, setEmailType] = useState(EMAIL_TYPES[0]);
  const [deleting, setDeleting] = useState(false);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (order) setStatus(order.status);
  }, [order]);

  if (loading) return <Spinner />;
  if (error || !order) return <ErrorBox error={error} />;

  const save = async (body) => {
    setBusy(true);
    try {
      const { data } = await api.put(`/v1/admin/orders/${id}`, body);
      if (data?.id) setData(data);
      else reload();
      toast.success(t("admin.saved"));
      return true;
    } catch (err) {
      toast.error(errMsg(err));
      return false;
    } finally {
      setBusy(false);
    }
  };

  const sendEmail = async () => {
    try {
      await api.post(`/v1/admin/orders/${id}/emails`, { type: emailType });
      toast.success(t("admin.orders.emailSent"));
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const remove = async () => {
    setDeleting(false);
    try {
      await api.delete(`/v1/admin/orders/${id}`);
      toast.success(t("admin.deleted"));
      window.history.back();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className="stack-lg">
      <PageHead title={t("admin.orders.title", { number: order.number || order.id })} subtitle={`${t("admin.orders.paidVia", { method: order.paymentMethodTitle || order.paymentMethod || "—" })} · ${formatDateTime(order.createdAt)}`}>
        <Link to="/admin/orders" className="btn btn-secondary btn-sm">
          {t("common.back")}
        </Link>
      </PageHead>
      <div className="admin-columns">
        <div className="stack-lg">
          <section className="card">
            <div className="form-grid">
              <div className="field">
                <label className="field-label" htmlFor="order-status">
                  {t("order.status")}
                </label>
                <div className="row">
                  <select id="order-status" className="input" value={status} onChange={(e) => setStatus(e.target.value)}>
                    {ORDER_STATUSES.map((s) => (
                      <option key={s} value={s}>
                        {t(`status.${s}`)}
                      </option>
                    ))}
                  </select>
                  <button type="button" className="btn btn-primary" disabled={busy || status === order.status} onClick={() => save({ status })}>
                    {t("common.update")}
                  </button>
                </div>
              </div>
              <div className="field">
                <span className="field-label">{t("admin.orders.customer")}</span>
                {order.customerId ? <Link to={`/admin/customers/${order.customerId}`}>{t("admin.orders.viewCustomer", { id: order.customerId })}</Link> : <span className="muted">{t("admin.orders.guest")}</span>}
                {order.customerIpAddress && <span className="small muted">IP {order.customerIpAddress}</span>}
              </div>
            </div>
            <div className="two-col mt">
              {["billing", "shipping"].map((kind) => (
                <section key={kind}>
                  <div className="section-head">
                    <h3>{t(`address.${kind}`)}</h3>
                    {editing !== kind && (
                      <button
                        type="button"
                        className="btn btn-secondary btn-sm"
                        onClick={() => {
                          setEditing(kind);
                          setDraft(order[kind] || {});
                        }}
                      >
                        {t("common.edit")}
                      </button>
                    )}
                  </div>
                  {editing === kind ? (
                    <form
                      className="stack"
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (await save({ [kind]: draft })) setEditing(null);
                      }}
                    >
                      <AddressForm value={draft} onChange={setDraft} withEmail={kind === "billing"} countries={countries} required={false} idPrefix={kind} />
                      <div className="row">
                        <button type="submit" className="btn btn-primary btn-sm" disabled={busy}>
                          {t("common.save")}
                        </button>
                        <button type="button" className="btn btn-secondary btn-sm" onClick={() => setEditing(null)}>
                          {t("common.cancel")}
                        </button>
                      </div>
                    </form>
                  ) : (
                    <AddressView address={order[kind]} />
                  )}
                </section>
              ))}
            </div>
            {order.customerNote && (
              <p className="mt">
                <strong>{t("checkout.orderNotes")}:</strong> {order.customerNote}
              </p>
            )}
          </section>

          <section className="card">
            <h2>{t("admin.orders.items")}</h2>
            <div className="table-wrap">
              <table className="table">
                <thead>
                  <tr>
                    <th scope="col">{t("cart.product")}</th>
                    <th scope="col">{t("product.sku")}</th>
                    <th scope="col" className="num">
                      {t("cart.quantity")}
                    </th>
                    <th scope="col" className="num">
                      {t("cart.total")}
                    </th>
                    <th scope="col" className="num">
                      {t("admin.orders.tax")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {(order.lineItems || []).map((l) => (
                    <tr key={l.id}>
                      <td>
                        <Link to={`/admin/products/${l.productId}`}>{l.name}</Link>
                        {(l.meta || []).length > 0 && <div className="small muted">{l.meta.map((m) => `${m.key}: ${m.value}`).join(", ")}</div>}
                      </td>
                      <td>{l.sku}</td>
                      <td className="num">{l.quantity}</td>
                      <td className="num">
                        {Number(l.subtotal) !== Number(l.total) && <del className="muted small">{money(l.subtotal)} </del>}
                        {money(l.total)}
                      </td>
                      <td className="num">{money(l.totalTax)}</td>
                    </tr>
                  ))}
                  {(order.shippingLines || []).map((s) => (
                    <tr key={`s${s.id}`}>
                      <td colSpan={3}>{s.methodTitle}</td>
                      <td className="num">{money(s.total)}</td>
                      <td className="num">{money(s.totalTax)}</td>
                    </tr>
                  ))}
                  {(order.couponLines || []).map((c) => (
                    <tr key={`c${c.id}`}>
                      <td colSpan={3}>{t("cart.coupon", { code: c.code })}</td>
                      <td className="num">-{money(c.discount)}</td>
                      <td className="num">-{money(c.discountTax)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <OrderTotals order={order} />
            <div className="row end">
              <button type="button" className="btn btn-secondary" onClick={() => setRefundOpen(true)} disabled={Number(order.total) - Number(order.totalRefunded || 0) <= 0}>
                {t("admin.orders.refund")}
              </button>
            </div>
          </section>
        </div>

        <aside className="stack-lg">
          <section className="card stack">
            <h2>{t("admin.orders.actions")}</h2>
            <div className="field">
              <label className="field-label" htmlFor="email-type">
                {t("admin.orders.resendEmail")}
              </label>
              <select id="email-type" className="input" value={emailType} onChange={(e) => setEmailType(e.target.value)}>
                {EMAIL_TYPES.map((e) => (
                  <option key={e} value={e}>
                    {t(`admin.orders.emailType.${e}`)}
                  </option>
                ))}
              </select>
            </div>
            <button type="button" className="btn btn-secondary btn-sm" onClick={sendEmail}>
              {t("admin.orders.send")}
            </button>
            <button type="button" className="link-btn danger" onClick={() => setDeleting(true)}>
              {t("admin.orders.delete")}
            </button>
            <StatusBadge status={order.status} label={t(`status.${order.status}`)} />
          </section>
          <Notes orderId={order.id} />
        </aside>
      </div>
      <RefundModal
        order={order}
        show={refundOpen}
        onClose={() => setRefundOpen(false)}
        onDone={() => {
          setRefundOpen(false);
          reload();
        }}
      />
      <ConfirmModal show={deleting} title={t("admin.orders.delete")} body={t("admin.orders.deleteBody")} onConfirm={remove} onCancel={() => setDeleting(false)} confirmLabel={t("common.delete")} />
    </div>
  );
};

export default OrderDetail;
