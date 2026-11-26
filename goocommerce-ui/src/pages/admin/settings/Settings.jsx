import { NavLink, useParams } from "react-router-dom";
import { useI18n } from "../../../i18n/I18nContext";
import { PageHead } from "../../../components/admin";
import OptionsTab from "./OptionsTab";
import TaxTab from "./TaxTab";
import ShippingTab from "./ShippingTab";
import PaymentsTab from "./PaymentsTab";
import EmailsTab from "./EmailsTab";

export const OPTION_TABS = ["general", "products", "accounts", "advanced"];
const TABS = [...OPTION_TABS.slice(0, 2), "tax", "shipping", "payments", OPTION_TABS[2], "emails", OPTION_TABS[3]];

const Settings = () => {
  const { tab: rawTab } = useParams();
  const tab = rawTab || "general";
  const { t } = useI18n();
  return (
    <div className="stack-lg">
      <PageHead title={t("admin.nav.settings")} />
      <nav className="tabs tabs-scroll" aria-label={t("admin.nav.settings")}>
        {TABS.map((x) => (
          <NavLink key={x} to={`/admin/settings/${x}`} className={({ isActive }) => `tab ${isActive || (x === "general" && !rawTab) ? "is-active" : ""}`}>
            {t(`settings.tabs.${x}`)}
          </NavLink>
        ))}
      </nav>
      {OPTION_TABS.includes(tab) && <OptionsTab group={tab} />}
      {tab === "tax" && <TaxTab />}
      {tab === "shipping" && <ShippingTab />}
      {tab === "payments" && <PaymentsTab />}
      {tab === "emails" && <EmailsTab />}
    </div>
  );
};

export default Settings;
