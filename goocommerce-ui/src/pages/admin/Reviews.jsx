import { useState } from "react";
import { Link } from "react-router-dom";
import { toast } from "react-toastify";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useI18n } from "../../i18n/I18nContext";
import { PageHead } from "../../components/admin";
import { ConfirmModal, ErrorBox, Modal, Pagination, Spinner, Stars, Tabs, TextArea } from "../../components/ui";

const STATUSES = ["", "hold", "approved", "spam", "trash"];

const Reviews = () => {
  const { t, errMsg, formatDateTime } = useI18n();
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [replying, setReplying] = useState(null);
  const [reply, setReply] = useState("");
  const [deleting, setDeleting] = useState(null);
  const { data, loading, error, reload } = useAsync(
    async () => normalizePaged((await api.get("/v1/admin/reviews", { params: { status: status || undefined, page, perPage: 20 } })).data, page),
    [status, page],
    { items: [], totalPages: 1 }
  );

  const act = async (r, next) => {
    try {
      await api.put(`/v1/admin/reviews/${r.id}`, { status: next });
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  const sendReply = async (e) => {
    e.preventDefault();
    try {
      await api.post(`/v1/admin/reviews/${replying.id}/reply`, { review: reply });
      toast.success(t("admin.reviews.replied"));
      setReplying(null);
      setReply("");
      reload();
    } catch (err) {
      toast.error(errMsg(err));
    }
  };

  return (
    <div className="stack">
      <PageHead title={t("admin.nav.reviews")} />
      <Tabs
        tabs={STATUSES.map((s) => ({ id: s, label: s ? t(`admin.reviews.status.${s}`) : t("common.all") }))}
        active={status}
        onChange={(s) => {
          setStatus(s);
          setPage(1);
        }}
      />
      <ErrorBox error={error} />
      {loading ? (
        <Spinner />
      ) : data.items.length === 0 ? (
        <p className="muted">{t("admin.reviews.none")}</p>
      ) : (
        <ul className="plain-list review-admin-list">
          {data.items.map((r) => (
            <li key={r.id} className={`card review-admin status-${r.status}`}>
              <div className="section-head">
                <div>
                  <strong>{r.reviewer}</strong> <span className="muted small">{r.reviewerEmail}</span>
                  {r.verified && <span className="badge badge-completed">{t("reviews.verified")}</span>}
                  <div className="small muted">
                    {formatDateTime(r.createdAt)} · {t("admin.reviews.on")} <Link to={`/admin/products/${r.productId}`}>{r.productName || `#${r.productId}`}</Link>
                  </div>
                </div>
                <span className="badge">{t(`admin.reviews.status.${r.status}`)}</span>
              </div>
              {Number(r.rating) > 0 && <Stars rating={r.rating} />}
              <p className="pre-line">{r.review}</p>
              <div className="row wrap">
                {r.status !== "approved" ? (
                  <button type="button" className="btn btn-secondary btn-sm" onClick={() => act(r, "approved")}>
                    {t("admin.reviews.approve")}
                  </button>
                ) : (
                  <button type="button" className="btn btn-secondary btn-sm" onClick={() => act(r, "hold")}>
                    {t("admin.reviews.unapprove")}
                  </button>
                )}
                <button type="button" className="btn btn-secondary btn-sm" onClick={() => setReplying(r)}>
                  {t("admin.reviews.reply")}
                </button>
                {r.status !== "spam" && (
                  <button type="button" className="btn btn-secondary btn-sm" onClick={() => act(r, "spam")}>
                    {t("admin.reviews.spam")}
                  </button>
                )}
                {r.status !== "trash" ? (
                  <button type="button" className="link-btn danger" onClick={() => act(r, "trash")}>
                    {t("admin.trash")}
                  </button>
                ) : (
                  <button type="button" className="link-btn danger" onClick={() => setDeleting(r)}>
                    {t("admin.deletePermanently")}
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
      <Pagination page={page} totalPages={data.totalPages} onChange={setPage} />
      <Modal show={!!replying} title={t("admin.reviews.replyTo", { name: replying?.reviewer || "" })} onClose={() => setReplying(null)}>
        <form className="stack" onSubmit={sendReply}>
          <TextArea label={t("admin.reviews.yourReply")} value={reply} onChange={setReply} rows={4} required />
          <div className="row end">
            <button type="submit" className="btn btn-primary" disabled={!reply.trim()}>
              {t("admin.reviews.reply")}
            </button>
          </div>
        </form>
      </Modal>
      <ConfirmModal
        show={!!deleting}
        title={t("admin.deletePermanently")}
        body={t("admin.reviews.deleteBody")}
        confirmLabel={t("common.delete")}
        onConfirm={async () => {
          const r = deleting;
          setDeleting(null);
          try {
            await api.delete(`/v1/admin/reviews/${r.id}`);
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

export default Reviews;
