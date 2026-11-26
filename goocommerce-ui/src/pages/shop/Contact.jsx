import { useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useAuth } from "../../context/AuthContext";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { ErrorBox, TextArea, TextField } from "../../components/ui";

const Contact = () => {
  const { user } = useAuth();
  const { shopName, appearance } = useShop();
  const { t, errMsg } = useI18n();
  const [form, setForm] = useState({ firstname: user?.name || "", name: user?.surname || "", email: user?.email || "", subject: "", message: "" });
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [error, setError] = useState(null);
  useSeo({ title: `${t("contact.title")} – ${shopName}`, description: t("contact.intro") });
  const set = (k) => (v) => setForm({ ...form, [k]: v });

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      await api.post("/v1/contact", form);
      setSent(true);
      toast.success(t("contact.sent"));
    } catch (err) {
      setError(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  const c = appearance?.contact || {};
  return (
    <div className="container narrow">
      <h1>{t("contact.title")}</h1>
      <p className="muted">{t("contact.intro")}</p>
      {(c.phone || c.email || c.address) && (
        <p className="small">
          {[c.phone, c.email, c.address].filter(Boolean).join(" · ")}
        </p>
      )}
      {sent ? (
        <p className="notice notice-success">{t("contact.sentBody")}</p>
      ) : (
        <form className="card stack" onSubmit={submit}>
          <div className="form-grid">
            <TextField label={t("contact.firstname")} value={form.firstname} onChange={set("firstname")} required autoComplete="given-name" />
            <TextField label={t("contact.name")} value={form.name} onChange={set("name")} required autoComplete="family-name" />
          </div>
          <TextField label={t("auth.email")} type="email" value={form.email} onChange={set("email")} required autoComplete="email" />
          <TextField label={t("contact.subject")} value={form.subject} onChange={set("subject")} required />
          <TextArea label={t("contact.message")} value={form.message} onChange={set("message")} required rows={6} />
          <ErrorBox error={error} />
          <button type="submit" className="btn btn-primary" disabled={busy}>
            {t("contact.send")}
          </button>
        </form>
      )}
    </div>
  );
};

export default Contact;
