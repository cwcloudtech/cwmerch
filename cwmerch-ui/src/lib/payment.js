// Online payments (Stripe, Konnect, Flouci): helpers shared by the checkout,
// the payment-return page and the "Pay now" buttons.

export const ONLINE_PROVIDERS = ["stripe", "konnect", "flouci"];

const STRIPE_JS = "https://js.stripe.com/v3/";
let stripePromise = null;

// loadStripeJs injects Stripe.js once and resolves window.Stripe.
export const loadStripeJs = () => {
  if (typeof window !== "undefined" && window.Stripe) return Promise.resolve(window.Stripe);
  if (stripePromise) return stripePromise;
  stripePromise = new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[src="${STRIPE_JS}"]`);
    const script = existing || document.createElement("script");
    const done = () => (window.Stripe ? resolve(window.Stripe) : reject(new Error("Stripe.js unavailable")));
    script.addEventListener("load", done);
    script.addEventListener("error", () => {
      stripePromise = null;
      script.remove();
      reject(new Error("Stripe.js could not be loaded"));
    });
    if (!existing) {
      script.src = STRIPE_JS;
      script.async = true;
      document.head.appendChild(script);
    }
  });
  return stripePromise;
};

// paymentReturnPath is where providers send the browser back (and the page polling the status).
export const paymentReturnPath = (orderId, orderKey) => `/checkout/payment/${orderId}?key=${encodeURIComponent(orderKey || "")}`;

// isOnlineMethod tells whether an order's payment method is an online gateway.
export const isOnlineMethod = (paymentMethod, gateways = []) => {
  if (!paymentMethod) return false;
  const gw = gateways.find((g) => g.id === paymentMethod);
  if (gw) return gw.type === "online";
  return ONLINE_PROVIDERS.includes(paymentMethod);
};

// canPayNow: a pending order paid with an online method can be (re)paid.
export const canPayNow = (order, gateways) => !!order && order.status === "pending" && isOnlineMethod(order.paymentMethod, gateways);

// stripeAppearance maps the shop's CSS variables onto Stripe's Appearance API
// (Stripe needs concrete values, so they are read from the computed style).
export const stripeAppearance = () => {
  const css = getComputedStyle(document.documentElement);
  const v = (name, fallback) => (css.getPropertyValue(name) || "").trim() || fallback;
  return {
    theme: "stripe",
    variables: {
      colorPrimary: v("--cwmerch-primary", "#0a7fae"),
      colorBackground: v("--cwmerch-bg", "#ffffff"),
      colorText: v("--cwmerch-text", "#1e1e1e"),
      colorTextSecondary: v("--cwmerch-muted", "#6b6b76"),
      colorDanger: v("--cwmerch-sale", "#d63638"),
      fontFamily: v("--cwmerch-font-body", "system-ui, sans-serif"),
      borderRadius: v("--cwmerch-radius", "8px"),
    },
  };
};
