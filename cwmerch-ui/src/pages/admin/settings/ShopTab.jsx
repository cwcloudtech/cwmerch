import { useEffect, useState } from "react";
import { toast } from "react-toastify";
import { api } from "../../../api/client";
import { useShop } from "../../../context/ShopContext";
import { useI18n } from "../../../i18n/I18nContext";
import { ErrorBox, Spinner, TextArea, Toggle } from "../../../components/ui";

// ShopTab: the shop switch (GET/PUT /v1/admin/shop-config). With the public
// shop off, cwmerch runs as an internal information system (back office only).
const ShopTab = () => {
  const { t, errMsg } = useI18n();
  const { reload: reloadShop } = useShop();
  const [config, setConfig] = useState(null);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api
      .get("/v1/admin/shop-config")
      .then(({ data }) => setConfig({ shopEnabled: true, registrationEnabled: true, closedMessage: "", ...data }))
      .catch(setError);
  }, []);

  if (!config) return error ? <ErrorBox error={error} /> : <Spinner />;
  const set = (k) => (v) => setConfig((c) => ({ ...c, [k]: v }));

  const save = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      const { data } = await api.put("/v1/admin/shop-config", {
        shopEnabled: !!config.shopEnabled,
        registrationEnabled: !!config.registrationEnabled,
        closedMessage: config.closedMessage || "",
      });
      if (data && typeof data === "object") setConfig((c) => ({ ...c, ...data }));
      toast.success(t("admin.saved"));
      reloadShop();
    } catch (err) {
      toast.error(errMsg(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="stack-lg" onSubmit={save}>
      <section className="card stack settings-section">
        <h2>{t("shopConfig.title")}</h2>
        <p className="muted small">{t("shopConfig.intro")}</p>
        <Toggle checked={config.shopEnabled} onChange={set("shopEnabled")} label={t("shopConfig.shopEnabled")} hint={t("shopConfig.shopEnabledHint")} />
        {!config.shopEnabled && <p className="notice notice-info">{t("shopConfig.closedNotice")}</p>}
        <TextArea label={t("shopConfig.closedMessage")} hint={t("shopConfig.closedMessageHint")} value={config.closedMessage} onChange={set("closedMessage")} rows={4} />
        <Toggle checked={config.registrationEnabled} onChange={set("registrationEnabled")} label={t("shopConfig.registrationEnabled")} hint={t("shopConfig.registrationEnabledHint")} />
      </section>
      <div className="sticky-save">
        <button type="submit" className="btn btn-primary" disabled={busy}>
          {t("common.saveChanges")}
        </button>
      </div>
    </form>
  );
};

export default ShopTab;
