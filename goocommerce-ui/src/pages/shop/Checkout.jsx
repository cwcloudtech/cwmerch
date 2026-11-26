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

const Checkout = () => {
  const { items, coupons, cart, pricing, setCustomer, setShippingMethod, estimate, clear } = useCart();
  const { settings, money, cartIncl, shopName } = useShop();
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
    const gws = settings.paymentGateways || [];
    if (!paymentMethod && gws.length) setPaymentMethod(gws[0].id);
  }, [settings.paymentGateways, paymentMethod]);

  // Re-price with the destination so shipping rates and taxes follow the address.
  const dest = shipToDifferent ? shipping : billing;
  useEffect(() => {
    const handle = setTimeout(() => setCustomer({ country: dest.country, state: dest.state, postcode: dest.postcode, city: dest.city }), 400);
    return () => clearTimeout(handle);
  }, [dest.country, dest.state, dest.postcode, dest.city, setCustomer]);

  if (items.length === 0) return <Navigate to="/cart" replace />;

  const guestBlocked = !user && !settings.guestCheckout && !(settings.signupAtCheckout && createAccount);

  const placeOrder = async (e) => {
    e.preventDefault();
    setError(null);
    if (guestBlocked) {
      setError(t("checkout.loginRequired"));
      return;
    }
    if (!paymentMethod && (settings.paymentGateways || []).length) {
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
      navigate(`/checkout/order-received/${order.id}?key=${encodeURIComponent(order.orderKey || "")}`, {
        replace: true,
        state: { order, paymentInstructions: data.paymentInstructions },
      });
    } catch (err) {
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
            <AddressForm value={billing} onChange={setBilling} withEmail idPrefix="billing" />
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
          {!user && settings.signupAtCheckout && (
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
          <section>
            <h3>{t("checkout.paymentMethod")}</h3>
            {(settings.paymentGateways || []).length === 0 ? (
              <p className="muted small">{t("checkout.noPayment")}</p>
            ) : (
              <fieldset className="radio-list">
                <legend className="sr-only">{t("checkout.paymentMethod")}</legend>
                {settings.paymentGateways.map((g) => (
                  <div key={g.id} className="payment-method">
                    <label className="radio-row">
                      <input type="radio" name="payment" value={g.id} checked={paymentMethod === g.id} onChange={() => setPaymentMethod(g.id)} />
                      <span>{g.title}</span>
                    </label>
                    {paymentMethod === g.id && g.description && <p className="payment-desc small">{g.description}</p>}
                  </div>
                ))}
              </fieldset>
            )}
          </section>
          <button type="submit" className="btn btn-primary btn-lg btn-block" disabled={busy || pricing || guestBlocked}>
            {busy ? t("checkout.placing") : t("checkout.placeOrder")}
          </button>
        </aside>
      </form>
    </div>
  );
};

export default Checkout;
