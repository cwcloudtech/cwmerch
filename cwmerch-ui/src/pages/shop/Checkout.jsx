import { useEffect, useState } from "react";
import { Link, Navigate, useNavigate } from "react-router-dom";
import { toast } from "react-toastify";
import { api } from "../../api/client";
import { useCart } from "../../context/CartContext";
import { useShop } from "../../context/ShopContext";
import { useAuth } from "../../context/AuthContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import AddressForm, { EMPTY_ADDRESS } from "../../components/AddressForm";
import { ErrorBox, TextArea, TextField, Toggle } from "../../components/ui";
import { CartNotices, CartTotals, ShippingRates } from "./Cart";
import { paymentReturnPath } from "../../lib/payment";

// PHONE_RE: 8 to 15 digits, with + . - ( ) and spaces allowed (as the API checks).
const PHONE_CHARS = /^[+\d.\-()\s]+$/;
export const validPhone = (phone) => {
  const v = String(phone || "").trim();
  if (!PHONE_CHARS.test(v)) return false;
  const digits = v.replace(/\D/g, "").length;
  return digits >= 8 && digits <= 15;
};

const Checkout = () => {
  const { items, coupons, cart, pricing, setCustomer, setShippingMethod, estimate, clear } = useCart();
  const { settings, money, cartIncl, shopName, registrationEnabled } = useShop();
  const { user, loginWithToken } = useAuth();
  const { t, errMsg } = useI18n();
  const navigate = useNavigate();
  const [billing, setBilling] = useState({ ...EMPTY_ADDRESS, country: (settings.defaultCountry || "").split(":")[0] });
  const [shipping, setShipping] = useState({ ...EMPTY_ADDRESS, country: (settings.defaultCountry || "").split(":")[0] });
  const [shipToDifferent, setShipToDifferent] = useState(false);
  const [paymentMethod, setPaymentMethod] = useState("");
  const [customerNote, setCustomerNote] = useState("");
  const [createAccount, setCreateAccount] = useState(false);
  const [accountPassword, setAccountPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);
  const [phoneError, setPhoneError] = useState("");
  useSeo({ title: `${t("checkout.title")} – ${shopName}`, noindex: true });

  // Prefill from the account's saved addresses.
  useEffect(() => {
    if (!user) return;
    api
      .get("/v1/users/me/addresses")
      .then(({ data }) => {
        if (data?.billing) setBilling((b) => ({ ...b, ...Object.fromEntries(Object.entries(data.billing).filter(([, v]) => v)) }));
        if (data?.shipping) setShipping((s) => ({ ...s, ...Object.fromEntries(Object.entries(data.shipping).filter(([, v]) => v)) }));
        if (!data?.billing?.email && user.email) setBilling((b) => ({ ...b, email: b.email || user.email }));
      })
      .catch(() => {});
  }, [user]);

  // The shop settings may arrive after mount: default empty countries then.
  useEffect(() => {
    if (!settings.defaultCountry) return;
    const country = settings.defaultCountry.split(":")[0];
    setBilling((b) => (b.country ? b : { ...b, country }));
    setShipping((s) => (s.country ? s : { ...s, country }));
  }, [settings.defaultCountry]);

  useEffect(() => {
    // With phone confirmation the method stays optional: no default pick.
    const gws = settings.paymentGateways || [];
    if (!settings.phoneConfirmation && !paymentMethod && gws.length) setPaymentMethod(gws[0].id);
  }, [settings.paymentGateways, settings.phoneConfirmation, paymentMethod]);

  // Re-price with the destination so shipping rates and taxes follow the address.
  const dest = shipToDifferent ? shipping : billing;
  useEffect(() => {
    const handle = setTimeout(() => setCustomer({ country: dest.country, state: dest.state, postcode: dest.postcode, city: dest.city }), 400);
    return () => clearTimeout(handle);
  }, [dest.country, dest.state, dest.postcode, dest.city, setCustomer]);

  if (items.length === 0) return <Navigate to="/cart" replace />;

  const phoneConfirmation = !!settings.phoneConfirmation;
  const gateways = settings.paymentGateways || [];
  const signupAllowed = settings.signupAtCheckout && registrationEnabled;
  const guestBlocked = !user && !settings.guestCheckout && !(signupAllowed && createAccount);

  const placeOrder = async (e) => {
    e.preventDefault();
    setError(null);
    if (guestBlocked) {
      setError(t("checkout.loginRequired"));
      return;
    }
    if (phoneConfirmation && !validPhone(billing.phone)) {
      setPhoneError(billing.phone?.trim() ? t("phoneConfirm.invalidPhone") : t("errors.phoneRequired"));
      document.getElementById("billing-phone-error")?.scrollIntoView({ behavior: "smooth", block: "center" });
      return;
    }
    // With phone confirmation the payment method is optional (offline gateways only).
    if (!phoneConfirmation && !paymentMethod && gateways.length) {
      setError(t("checkout.choosePayment"));
      return;
    }
    setBusy(true);
    try {
      const body = {
        items,
        coupons,
        ...(cart?.chosenShippingMethod || estimate.shippingMethod ? { shippingMethod: cart?.chosenShippingMethod || estimate.shippingMethod } : {}),
        billing,
        shipping: shipToDifferent ? shipping : { ...billing, email: undefined },
        shipToDifferentAddress: shipToDifferent,
        paymentMethod,
        customerNote,
        createAccount: !user && createAccount,
        accountPassword: !user && createAccount ? accountPassword : "",
      };
      const { data } = await api.post("/v1/shop/checkout", body);
      if (data.confirmation) {
        if (data.token && !user) {
          try {
            await loginWithToken(data.token);
          } catch {
            /* the order is placed even if the auto-login fails */
          }
        }
        const placed = data.order || data;
        clear();
        toast.success(t("checkout.placed"));
        navigate(`/checkout/order-received/${placed.id}?key=${encodeURIComponent(placed.orderKey || "")}`, {
          replace: true,
          state: { order: placed, confirmation: data.confirmation, paymentInstructions: data.paymentInstructions },
        });
        return;
      }
      const order = data.order || data;
      if (data.token && !user) {
        try {
          await loginWithToken(data.token);
        } catch {
          /* the order is placed even if the auto-login fails */
        }
      }
      clear();
      toast.success(t("checkout.placed"));
      if (data.payment) {
        // Online gateway: the order is pending until the provider confirms it.
        navigate(paymentReturnPath(order.id, order.orderKey), { replace: true, state: { payment: data.payment, order } });
        return;
      }
      navigate(`/checkout/order-received/${order.id}?key=${encodeURIComponent(order.orderKey || "")}`, {
        replace: true,
        state: { order, paymentInstructions: data.paymentInstructions },
      });
    } catch (err) {
      if (err?.response?.data?.i18n_code === "errors.phoneRequired") setPhoneError(errMsg(err));
      setError(errMsg(err));
      window.scrollTo({ top: 0, behavior: "smooth" });
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="container checkout-page">
      <h1>{t("checkout.title")}</h1>
      {!user && (
        <p className="notice notice-info">
          {t("checkout.returning")} <Link to="/my-account?redirect=/checkout">{t("checkout.clickToLogin")}</Link>
        </p>
      )}
      <CartNotices notices={cart?.notices} />
      <ErrorBox error={error} />
      <form className="checkout-layout" onSubmit={placeOrder}>
        <div className="stack-lg">
          <section className="card">
            <h2>{t("checkout.billingDetails")}</h2>
            <AddressForm
              value={billing}
              onChange={(b) => {
                setBilling(b);
                if (phoneError && b.phone !== billing.phone) setPhoneError("");
              }}
              withEmail
              idPrefix="billing"
              phoneRequired={phoneConfirmation}
              phoneHint={phoneConfirmation ? t("phoneConfirm.phoneHint") : undefined}
              phoneError={phoneError}
            />
          </section>
          {cart?.needsShipping !== false && (
            <section className="card">
              <Toggle checked={shipToDifferent} onChange={setShipToDifferent} label={t("checkout.shipToDifferent")} />
              {shipToDifferent && (
                <div className="mt">
                  <AddressForm value={shipping} onChange={setShipping} idPrefix="shipping" />
                </div>
              )}
            </section>
          )}
          {!user && signupAllowed && (
            <section className="card">
              <Toggle checked={createAccount} onChange={setCreateAccount} label={t("checkout.createAccount")} />
              {createAccount && (
                <TextField label={t("auth.password")} type="password" value={accountPassword} onChange={setAccountPassword} required autoComplete="new-password" />
              )}
            </section>
          )}
          {guestBlocked && (
            <p className="notice notice-error" role="alert">
              {t("checkout.loginRequired")} <Link to="/my-account?redirect=/checkout">{t("nav.signIn")}</Link>
            </p>
          )}
          <section className="card">
            <TextArea label={t("checkout.orderNotes")} value={customerNote} onChange={setCustomerNote} rows={3} placeholder={t("checkout.orderNotesPlaceholder")} />
          </section>
        </div>
        <aside className="card checkout-review" aria-busy={pricing}>
          <h2>{t("checkout.yourOrder")}</h2>
          <table className="table">
            <thead>
              <tr>
                <th scope="col">{t("cart.product")}</th>
                <th scope="col" className="num">
                  {t("cart.subtotal")}
                </th>
              </tr>
            </thead>
            <tbody>
              {(cart?.items || []).map((l) => (
                <tr key={l.key}>
                  <td>
                    {l.name} <strong>× {l.quantity}</strong>
                    {(l.attributes || []).length > 0 && <div className="small muted">{l.attributes.map((a) => `${a.name}: ${a.option}`).join(", ")}</div>}
                  </td>
                  <td className="num">{money(Number(l.subtotal || 0) + (cartIncl ? Number(l.subtotalTax || 0) : 0))}</td>
                </tr>
              ))}
            </tbody>
          </table>
          <CartTotals cart={cart} showShipping={false} />
          {cart?.needsShipping && (
            <section>
              <h3>{t("cart.shippingMethod")}</h3>
              <ShippingRates cart={cart} onChange={setShippingMethod} name="checkout-shipping" />
            </section>
          )}
          {cart?.needsShipping && Number(cart?.shippingTotal) > 0 && (
            <p className="small muted">
              {t("cart.shipping")}: {money(Number(cart.shippingTotal) + (cartIncl ? Number(cart.shippingTax || 0) : 0))}
            </p>
          )}
          {phoneConfirmation && <p className="notice notice-info small">{t("phoneConfirm.checkoutNotice")}</p>}
          {!(phoneConfirmation && gateways.length === 0) && (
          <section>
            <h3>{t("checkout.paymentMethod")}{phoneConfirmation && <span className="muted small"> ({t("phoneConfirm.optional")})</span>}</h3>
            {gateways.length === 0 ? (
              <p className="muted small">{t("checkout.noPayment")}</p>
            ) : (
              <fieldset className="radio-list">
                <legend className="sr-only">{t("checkout.paymentMethod")}</legend>
                {phoneConfirmation && (
                  <div className="payment-method">
                    <label className="radio-row">
                      <input type="radio" name="payment" value="" checked={paymentMethod === ""} onChange={() => setPaymentMethod("")} />
                      <span>{t("phoneConfirm.payLater")}</span>
                    </label>
                  </div>
                )}
                {gateways.map((g) => (
                  <div key={g.id} className="payment-method">
                    <label className="radio-row">
                      <input type="radio" name="payment" value={g.id} checked={paymentMethod === g.id} onChange={() => setPaymentMethod(g.id)} />
                      <span>{g.title}</span>
                      {g.type === "online" && <span className="badge badge-online small">{t("checkout.onlinePayment")}</span>}
                    </label>
                    {paymentMethod === g.id && g.description && <p className="payment-desc small">{g.description}</p>}
                  </div>
                ))}
              </fieldset>
            )}
          </section>
          )}
          <button type="submit" className="btn btn-primary btn-lg btn-block" disabled={busy || pricing || guestBlocked}>
            {busy ? t("checkout.placing") : t("checkout.placeOrder")}
          </button>
        </aside>
      </form>
    </div>
  );
};

export default Checkout;
