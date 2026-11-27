import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { FiRefreshCw } from "react-icons/fi";
import { api } from "../../api/client";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import { useSeo } from "../../lib/seo";
import { canPayNow, loadStripeJs, paymentReturnPath, stripeAppearance } from "../../lib/payment";
import { ErrorBox, Spinner } from "../../components/ui";

// Poll delays (ms) for GET /v1/shop/orders/{id}/payment: ~20 s in all.
const POLL_DELAYS = [0, 1000, 1500, 2000, 3000, 4000, 4000, 4500];

// StripePayment mounts a Stripe Payment Element for the PaymentIntent's
// clientSecret and confirms it; Stripe then sends the browser to return_url.
export const StripePayment = ({ payment, orderId, orderKey, total }) => {
  const { t } = useI18n();
  const mountRef = useRef(null);
  const stripeRef = useRef(null);
  const elementsRef = useRef(null);
  const [ready, setReady] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    let cancelled = false;
    let element = null;
    setReady(false);
    setError(null);
    loadStripeJs()
      .then((Stripe) => {
        if (cancelled || !mountRef.current) return;
        const stripe = Stripe(payment.publishableKey);
        const elements = stripe.elements({ clientSecret: payment.clientSecret, appearance: stripeAppearance() });
        element = elements.create("payment", { layout: "tabs" });
        element.on("ready", () => !cancelled && setReady(true));
        element.on("loaderror", (e) => !cancelled && setError(e?.error?.message || t("payment.stripeUnavailable")));
        element.mount(mountRef.current);
        stripeRef.current = stripe;
        elementsRef.current = elements;
      })
      .catch(() => !cancelled && setError(t("payment.stripeUnavailable")));
    return () => {
      cancelled = true;
      try {
        element?.destroy();
      } catch {
        /* already gone */
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [payment.clientSecret, payment.publishableKey]);

  const pay = async (e) => {
    e.preventDefault();
    if (!stripeRef.current || !elementsRef.current) return;
    setBusy(true);
    setError(null);
    const { error: err } = await stripeRef.current.confirmPayment({
      elements: elementsRef.current,
      confirmParams: { return_url: `${window.location.origin}${paymentReturnPath(orderId, orderKey)}` },
    });
    // Only reached when the confirmation failed immediately (else Stripe redirects).
    if (err) setError(err.message || t("payment.failed"));
    setBusy(false);
  };

  return (
    <form className="card stack stripe-payment" onSubmit={pay}>
      <h2>{t("payment.cardDetails")}</h2>
      <div ref={mountRef} className="stripe-element" />
      {!ready && !error && <Spinner />}
      <ErrorBox error={error} />
      <button type="submit" className="btn btn-primary btn-lg btn-block" disabled={!ready || busy}>
        {busy ? t("payment.processing") : t("payment.payTotal", { total })}
      </button>
    </form>
  );
};

// runPayment performs a `payment` object from checkout / pay: redirect providers
// leave the page; Stripe answers "stripe" so the caller mounts StripePayment.
export const runPayment = (payment) => {
  if (!payment) return "none";
  if (payment.provider === "stripe" && payment.clientSecret) return "stripe";
  if (payment.redirectUrl) {
    window.location.assign(payment.redirectUrl);
    return "redirect";
  }
  return "invalid";
};

// PayNowButton (order-received, my-account order): shown for a pending order
// paid online; opens the payment page which asks POST /pay for a new payment.
export const PayNowButton = ({ order }) => {
  const { settings } = useShop();
  const { t } = useI18n();
  const navigate = useNavigate();
  if (!canPayNow(order, settings.paymentGateways)) return null;
  return (
    <div className="notice notice-info pay-now">
      <span>{t("payment.awaiting")}</span>
      <button type="button" className="btn btn-primary btn-sm" onClick={() => navigate(paymentReturnPath(order.id, order.orderKey), { state: { retry: true, order } })}>
        {t("payment.payNow")}
      </button>
    </div>
  );
};

// Payment is /checkout/payment/:orderId?key= — the provider's return URL. It
// either runs a fresh payment (state.payment from checkout / "Pay now") or
// polls the order's payment status until it is paid, failed or still pending.
const Payment = () => {
  const { orderId } = useParams();
  const [params] = useSearchParams();
  const key = params.get("key") || "";
  const location = useLocation();
  const navigate = useNavigate();
  const { money, shopName } = useShop();
  const { t, errMsg } = useI18n();
  useSeo({ title: `${t("payment.title")} – ${shopName}`, noindex: true });

  const initial = location.state?.payment || null;
  const [payment, setPayment] = useState(initial);
  const [order, setOrder] = useState(location.state?.order || null);
  // phase: pay (Stripe element shown) | redirecting | polling | paid | failed | pending | error
  const [phase, setPhase] = useState(initial ? "pay" : location.state?.retry ? "retrying" : "polling");
  const [error, setError] = useState(null);
  const [pollRun, setPollRun] = useState(0);
  const startedRef = useRef(false);

  const goReceived = useCallback(
    (o) => navigate(`/checkout/order-received/${orderId}?key=${encodeURIComponent(key)}`, { replace: true, state: { order: o } }),
    [navigate, orderId, key]
  );

  const start = useCallback(
    (p) => {
      setPayment(p);
      const how = runPayment(p);
      if (how === "stripe") setPhase("pay");
      else if (how === "redirect") setPhase("redirecting");
      else {
        setError(t("payment.invalid"));
        setPhase("error");
      }
    },
    [t]
  );

  const retry = useCallback(async () => {
    setError(null);
    setPhase("retrying");
    try {
      const { data } = await api.post(`/v1/shop/orders/${orderId}/pay`, null, { params: { key } });
      start(data?.payment || data);
    } catch (err) {
      setError(errMsg(err));
      setPhase("error");
    }
  }, [orderId, key, start, errMsg]);

  // A fresh payment handed over by checkout / "Pay now": run it once.
  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    if (initial) start(initial);
    else if (location.state?.retry) retry();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Poll the payment status with a growing delay.
  useEffect(() => {
    if (phase !== "polling") return undefined;
    let cancelled = false;
    let timer = null;
    const step = async (i) => {
      if (cancelled) return;
      try {
        const { data } = await api.get(`/v1/shop/orders/${orderId}/payment`, { params: { key } });
        if (cancelled) return;
        if (data?.order) setOrder(data.order);
        if (data?.status === "paid") {
          goReceived(data.order);
          return;
        }
        if (data?.status === "failed") {
          setPhase("failed");
          return;
        }
      } catch (err) {
        if (cancelled) return;
        setError(errMsg(err));
        setPhase("error");
        return;
      }
      if (i + 1 >= POLL_DELAYS.length) {
        setPhase("pending");
        return;
      }
      timer = setTimeout(() => step(i + 1), POLL_DELAYS[i + 1]);
    };
    timer = setTimeout(() => step(0), POLL_DELAYS[0]);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [phase, pollRun, orderId, key, goReceived, errMsg]);

  const refresh = () => {
    setError(null);
    setPhase("polling");
    setPollRun((n) => n + 1);
  };

  const total = order ? money(order.total) : "";

  return (
    <div className="container narrow payment-page">
      <h1>{t("payment.title")}</h1>
      {order && (
        <p className="muted">
          {t("payment.forOrder", { number: order.number || order.id })}
          {total && ` · ${total}`}
        </p>
      )}
      {phase === "pay" && payment && <StripePayment payment={payment} orderId={orderId} orderKey={key} total={total} />}
      {(phase === "redirecting" || phase === "retrying") && (
        <div className="card stack center">
          <Spinner />
          <p>{phase === "redirecting" ? t("payment.redirecting") : t("payment.preparing")}</p>
        </div>
      )}
      {phase === "polling" && (
        <div className="card stack center" aria-live="polite">
          <Spinner />
          <p>{t("payment.checking")}</p>
        </div>
      )}
      {phase === "failed" && (
        <div className="card stack">
          <p className="notice notice-error" role="alert">
            {t("payment.failedMessage")}
          </p>
          <div className="row">
            <button type="button" className="btn btn-primary" onClick={retry}>
              {t("payment.tryAgain")}
            </button>
          </div>
        </div>
      )}
      {phase === "pending" && (
        <div className="card stack">
          <p className="notice notice-info">{t("payment.pendingMessage")}</p>
          <div className="row wrap">
            <button type="button" className="btn btn-primary" onClick={refresh}>
              <FiRefreshCw aria-hidden="true" /> {t("payment.refresh")}
            </button>
            <button type="button" className="btn btn-secondary" onClick={retry}>
              {t("payment.payNow")}
            </button>
          </div>
        </div>
      )}
      {phase === "error" && (
        <div className="card stack">
          <ErrorBox error={error || t("payment.invalid")} />
          <div className="row wrap">
            <button type="button" className="btn btn-primary" onClick={refresh}>
              <FiRefreshCw aria-hidden="true" /> {t("payment.refresh")}
            </button>
            <button type="button" className="btn btn-secondary" onClick={retry}>
              {t("payment.tryAgain")}
            </button>
          </div>
        </div>
      )}
      <p className="small">
        <Link to={`/checkout/order-received/${orderId}?key=${encodeURIComponent(key)}`}>{t("payment.viewOrder")}</Link>
      </p>
    </div>
  );
};

export default Payment;
