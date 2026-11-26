import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { api } from "../api/client";

const CART_KEY = "goocommerce.cart";
const ESTIMATE_KEY = "goocommerce.cartEstimate";

const readJSON = (key, fallback) => {
  try {
    const raw = localStorage.getItem(key);
    return raw ? { ...fallback, ...JSON.parse(raw) } : fallback;
  } catch {
    return fallback;
  }
};
const writeJSON = (key, value) => {
  try {
    localStorage.setItem(key, JSON.stringify(value));
  } catch {
    /* storage unavailable */
  }
};

const EMPTY = { items: [], coupons: [] };
const EMPTY_ESTIMATE = { customer: { country: "", state: "", postcode: "", city: "" }, shippingMethod: "" };

const attrsKey = (attrs) => JSON.stringify((attrs || []).map((a) => [a.slug, a.option]).sort());
const sameLine = (a, b) =>
  Number(a.productId) === Number(b.productId) &&
  Number(a.variationId || 0) === Number(b.variationId || 0) &&
  (b.attributes === undefined || attrsKey(a.attributes) === attrsKey(b.attributes));

const CartContext = createContext(null);

// The cart lives in localStorage as {items:[{productId, variationId, quantity}], coupons:[]};
// every change is re-priced by POST /v1/shop/cart, which is the only source of totals.
export const CartProvider = ({ children }) => {
  const [state, setState] = useState(() => {
    const s = readJSON(CART_KEY, EMPTY);
    return { items: Array.isArray(s.items) ? s.items : [], coupons: Array.isArray(s.coupons) ? s.coupons : [] };
  });
  const [estimate, setEstimate] = useState(() => readJSON(ESTIMATE_KEY, EMPTY_ESTIMATE));
  const [cart, setCart] = useState(null);
  const [pricing, setPricing] = useState(false);
  const [error, setError] = useState(null);
  const seq = useRef(0);

  useEffect(() => writeJSON(CART_KEY, state), [state]);
  useEffect(() => writeJSON(ESTIMATE_KEY, estimate), [estimate]);

  const reprice = useCallback(async () => {
    const id = ++seq.current;
    if (state.items.length === 0) {
      setCart(null);
      setPricing(false);
      return null;
    }
    setPricing(true);
    try {
      const body = {
        items: state.items,
        coupons: state.coupons,
        customer: estimate.customer,
        ...(estimate.shippingMethod ? { shippingMethod: estimate.shippingMethod } : {}),
      };
      const { data } = await api.post("/v1/shop/cart", body);
      if (id !== seq.current) return data;
      setCart(data);
      setError(null);
      // Drop coupons the API refused (its notice explains why).
      const accepted = (data?.coupons || []).map((c) => String(c.code).toLowerCase());
      if (state.coupons.some((c) => !accepted.includes(c))) {
        setState((s) => ({ ...s, coupons: s.coupons.filter((c) => accepted.includes(c)) }));
      }
      return data;
    } catch (e) {
      if (id === seq.current) setError(e);
      return null;
    } finally {
      if (id === seq.current) setPricing(false);
    }
  }, [state, estimate]);

  useEffect(() => {
    const handle = setTimeout(reprice, 200);
    return () => clearTimeout(handle);
  }, [reprice]);

  // attributes: the options chosen for a variation's "any" attributes ([{slug, option}]).
  const addItem = useCallback((productId, variationId = 0, quantity = 1, soldIndividually = false, attributes = undefined) => {
    setState((s) => {
      const line = { productId: Number(productId), variationId: Number(variationId || 0), quantity: Number(quantity), ...(attributes?.length ? { attributes } : {}) };
      const existing = s.items.find((i) => sameLine(i, line));
      const items = existing
        ? s.items.map((i) => (sameLine(i, line) ? { ...i, quantity: soldIndividually ? 1 : i.quantity + line.quantity } : i))
        : [...s.items, line];
      return { ...s, items };
    });
  }, []);

  const setQuantity = useCallback((productId, variationId, quantity) => {
    const q = Math.max(0, Number(quantity) || 0);
    setState((s) => ({
      ...s,
      items:
        q === 0
          ? s.items.filter((i) => !sameLine(i, { productId, variationId }))
          : s.items.map((i) => (sameLine(i, { productId, variationId }) ? { ...i, quantity: q } : i)),
    }));
  }, []);

  const removeItem = useCallback((productId, variationId) => setQuantity(productId, variationId, 0), [setQuantity]);

  const applyCoupon = useCallback((code) => {
    const c = String(code || "").trim().toLowerCase();
    if (!c) return;
    setState((s) => (s.coupons.includes(c) ? s : { ...s, coupons: [...s.coupons, c] }));
  }, []);

  const removeCoupon = useCallback((code) => {
    const c = String(code || "").toLowerCase();
    setState((s) => ({ ...s, coupons: s.coupons.filter((x) => x !== c) }));
  }, []);

  const setCustomer = useCallback((customer) => {
    setEstimate((e) => {
      const next = { ...e.customer, ...customer };
      if (JSON.stringify(next) === JSON.stringify(e.customer)) return e;
      return { ...e, customer: next };
    });
  }, []);

  const setShippingMethod = useCallback((shippingMethod) => setEstimate((e) => ({ ...e, shippingMethod })), []);

  const clear = useCallback(() => {
    setState(EMPTY);
    setCart(null);
  }, []);

  const value = useMemo(
    () => ({
      items: state.items,
      coupons: state.coupons,
      estimate,
      cart,
      pricing,
      error,
      count: state.items.reduce((n, i) => n + Number(i.quantity || 0), 0),
      addItem,
      setQuantity,
      removeItem,
      applyCoupon,
      removeCoupon,
      setCustomer,
      setShippingMethod,
      reprice,
      clear,
    }),
    [state, estimate, cart, pricing, error, addItem, setQuantity, removeItem, applyCoupon, removeCoupon, setCustomer, setShippingMethod, reprice, clear]
  );

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
};

export const useCart = () => useContext(CartContext);
