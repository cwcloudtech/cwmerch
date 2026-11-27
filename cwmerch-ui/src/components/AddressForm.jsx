import { useShop } from "../context/ShopContext";
import { useI18n } from "../i18n/I18nContext";
import { Field, TextField } from "./ui";

export const EMPTY_ADDRESS = {
  firstName: "",
  lastName: "",
  company: "",
  address1: "",
  address2: "",
  city: "",
  state: "",
  postcode: "",
  country: "",
  email: "",
  phone: "",
};

// AddressForm edits an Address. `countries` defaults to the shop's selling
// countries ([{code, name, states}]); the state becomes a select when the
// country has states.
// phoneRequired / phoneHint / phoneError: checkout with phone confirmation.
const AddressForm = ({ value, onChange, withEmail = false, countries: countriesProp, required = true, idPrefix = "addr", phoneRequired = false, phoneHint, phoneError }) => {
  const { t } = useI18n();
  const shop = useShop();
  const countries = countriesProp || shop.countries;
  const a = { ...EMPTY_ADDRESS, ...(value || {}) };
  const set = (key) => (v) => onChange({ ...a, [key]: v, ...(key === "country" ? { state: "" } : {}) });
  const states = countries.find((c) => c.code === a.country)?.states || [];

  return (
    <div className="form-grid">
      <TextField label={t("address.firstName")} value={a.firstName} onChange={set("firstName")} required={required} autoComplete={`${idPrefix} given-name`} />
      <TextField label={t("address.lastName")} value={a.lastName} onChange={set("lastName")} required={required} autoComplete={`${idPrefix} family-name`} />
      <TextField className="span-2" label={t("address.company")} value={a.company} onChange={set("company")} autoComplete={`${idPrefix} organization`} />
      <Field className="span-2" label={t("address.country")} required={required}>
        {(id) => (
          <select id={id} className="input" value={a.country} onChange={(e) => set("country")(e.target.value)} required={required} autoComplete={`${idPrefix} country`}>
            <option value="">{t("address.selectCountry")}</option>
            {countries.map((c) => (
              <option key={c.code} value={c.code}>
                {c.name}
              </option>
            ))}
          </select>
        )}
      </Field>
      <TextField className="span-2" label={t("address.address1")} value={a.address1} onChange={set("address1")} required={required} autoComplete={`${idPrefix} address-line1`} />
      <TextField className="span-2" label={t("address.address2")} value={a.address2} onChange={set("address2")} autoComplete={`${idPrefix} address-line2`} />
      <TextField label={t("address.postcode")} value={a.postcode} onChange={set("postcode")} required={required} autoComplete={`${idPrefix} postal-code`} />
      <TextField label={t("address.city")} value={a.city} onChange={set("city")} required={required} autoComplete={`${idPrefix} address-level2`} />
      {states.length > 0 ? (
        <Field className="span-2" label={t("address.state")}>
          {(id) => (
            <select id={id} className="input" value={a.state} onChange={(e) => set("state")(e.target.value)}>
              <option value="">{t("address.selectState")}</option>
              {states.map((s) => (
                <option key={s.code} value={s.code}>
                  {s.name}
                </option>
              ))}
            </select>
          )}
        </Field>
      ) : (
        <TextField className="span-2" label={t("address.state")} value={a.state} onChange={set("state")} />
      )}
      {withEmail && (
        <>
          <TextField label={t("address.email")} type="email" value={a.email} onChange={set("email")} required={required} autoComplete="email" />
          <TextField
            label={t("address.phone")}
            type="tel"
            value={a.phone}
            onChange={set("phone")}
            autoComplete="tel"
            required={phoneRequired}
            hint={phoneError ? undefined : phoneHint}
            aria-invalid={phoneError ? true : undefined}
            aria-describedby={phoneError ? `${idPrefix}-phone-error` : undefined}
          />
          {phoneError && (
            <p id={`${idPrefix}-phone-error`} className="field-error span-2" role="alert">
              {phoneError}
            </p>
          )}
        </>
      )}
      {!withEmail && <TextField className="span-2" label={t("address.phone")} type="tel" value={a.phone} onChange={set("phone")} autoComplete={`${idPrefix} tel`} />}
    </div>
  );
};

export const AddressView = ({ address }) => {
  const { countryName, stateName } = useShop();
  const { t } = useI18n();
  const a = address || {};
  const lines = [
    [a.firstName, a.lastName].filter(Boolean).join(" "),
    a.company,
    a.address1,
    a.address2,
    [a.postcode, a.city].filter(Boolean).join(" "),
    stateName(a.country, a.state),
    countryName(a.country),
  ].filter(Boolean);
  if (lines.length === 0 && !a.email && !a.phone) return <p className="muted">{t("address.none")}</p>;
  return (
    <address className="address">
      {lines.map((l, i) => (
        <span key={i}>
          {l}
          <br />
        </span>
      ))}
      {a.email && (
        <span>
          <a href={`mailto:${a.email}`}>{a.email}</a>
          <br />
        </span>
      )}
      {a.phone && <span dir="ltr">{a.phone}</span>}
    </address>
  );
};

export default AddressForm;
