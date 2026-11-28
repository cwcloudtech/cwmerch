import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import { ErrorBox, Modal, Spinner } from "../ui";
import SeoPanel, { EMPTY_SEO } from "./SeoPanel";

const endpointOf = (kind, id) =>
  kind === "product" ? `/v1/admin/seo/products/${id}` : `/v1/admin/seo/terms/${kind === "tag" ? "product_tag" : "product_cat"}/${id}`;

// SeoEditModal edits a product's or term's SEO data on its own (the SEO
// overview, the categories and tags screens).
const SeoEditModal = ({ kind, id, name, onClose, onSaved }) => {
  const { t, locale, errMsg } = useI18n();
  const [meta, setMeta] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api
      .get(endpointOf(kind, id), { params: { lng: locale } })
      .then(({ data }) => setMeta({ ...EMPTY_SEO, ...data.meta }))
      .catch(setError);
  }, [kind, id, locale]);
  const save = async () => {
    setBusy(true);
    setError(null);
    try {
      await api.put(endpointOf(kind, id), meta, { params: { lng: locale } });
      toast.success(t("admin.saved"));
      onSaved?.();
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };
  return (
    <Modal show wide title={t("seo.editTitle", { name })} onClose={onClose}>
      <div className="stack">
        <ErrorBox error={error} />
        {meta ? <SeoPanel kind={kind} id={id} meta={meta} onChange={setMeta} /> : !error && <Spinner />}
        <div className="row end">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button type="button" className="btn btn-primary" onClick={save} disabled={busy || !meta}>
            {t("common.saveChanges")}
          </button>
        </div>
      </div>
    </Modal>
  );
};

export default SeoEditModal;
