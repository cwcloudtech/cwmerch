import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useI18n } from "../../i18n/I18nContext";
import AddressForm, { AddressView, EMPTY_ADDRESS } from "../../components/AddressForm";
import { ErrorBox, Spinner } from "../../components/ui";

const Addresses = () => {
  const { t, errMsg } = useI18n();
  const [data, setData] = useState(null);
  const [editing, setEditing] = useState(null);
  const [draft, setDraft] = useState(EMPTY_ADDRESS);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .get("/v1/users/me/addresses")
      .then(({ data: d }) => setData({ billing: { ...EMPTY_ADDRESS, ...d?.billing }, shipping: { ...EMPTY_ADDRESS, ...d?.shipping } }))
      .catch((e) => setError(e));
  }, []);

  if (!data) return error ? <ErrorBox error={error} /> : <Spinner />;

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const next = { ...data, [editing]: draft };
      const { data: saved } = await api.put("/v1/users/me/addresses", next);
      setData(saved?.billing ? { billing: { ...EMPTY_ADDRESS, ...saved.billing }, shipping: { ...EMPTY_ADDRESS, ...saved.shipping } } : next);
      setEditing(null);
      toast.success(t("account.addressSaved"));
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  if (editing) {
    return (
      <form className="stack" onSubmit={save}>
        <h2>{t(`address.${editing}`)}</h2>
        <AddressForm value={draft} onChange={setDraft} withEmail={editing === "billing"} idPrefix={editing} />
        <ErrorBox error={error} />
        <div className="row">
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("common.save")}
          </button>
          <button type="button" className="btn btn-secondary" onClick={() => setEditing(null)}>
            {t("common.cancel")}
          </button>
        </div>
      </form>
    );
  }

  return (
    <div className="stack">
      <p className="muted">{t("account.addressesIntro")}</p>
      <div className="two-col">
        {["billing", "shipping"].map((kind) => (
          <section key={kind} className="card">
            <div className="section-head">
              <h2>{t(`address.${kind}`)}</h2>
              <button
                type="button"
                className="btn btn-secondary btn-sm"
                onClick={() => {
                  setDraft(data[kind]);
                  setEditing(kind);
                }}
              >
                {t("common.edit")}
              </button>
            </div>
            <AddressView address={data[kind]} />
          </section>
        ))}
      </div>
    </div>
  );
};

export default Addresses;
