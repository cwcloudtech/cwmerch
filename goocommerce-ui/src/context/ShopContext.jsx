import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { api, asArray } from "../api/client";
import { formatMoney } from "../lib/money";
import { applyAppearance } from "../lib/appearance";

const DEFAULT_SETTINGS = {
  name: "goocommerce",
  currency: "EUR",
  currencySymbol: "€",
  currencyPosition: "right_space",
  thousandSeparator: " ",
  decimalSeparator: ",",
  decimals: 2,
  calcTaxes: false,
  taxDisplayShop: "incl",
  taxDisplayCart: "incl",
  enableCoupons: true,
  enableReviews: true,
  enableReviewRating: true,
  reviewRatingRequired: false,
  guestCheckout: true,
  signupAtCheckout: true,
  paymentGateways: [],
  contactEnabled: true,
  oidcProviders: [],
};

const ShopContext = createContext(null);

export const ShopProvider = ({ children }) => {
  const [settings, setSettings] = useState(DEFAULT_SETTINGS);
  const [loaded, setLoaded] = useState(false);
  const [categories, setCategories] = useState([]);
  const [countries, setCountries] = useState([]);
  const [appearance, setAppearance] = useState(null);

  const reload = useCallback(async () => {
    const [s, c, k, a] = await Promise.allSettled([
      api.get("/v1/shop/settings"),
      api.get("/v1/shop/categories"),
      api.get("/v1/shop/countries"),
      api.get("/v1/shop/appearance"),
    ]);
    if (a.status === "fulfilled" && a.value.data) {
      setAppearance(a.value.data);
      applyAppearance(a.value.data);
    } else {
      applyAppearance(null);
    }
    if (s.status === "fulfilled") setSettings({ ...DEFAULT_SETTINGS, ...s.value.data });
    if (c.status === "fulfilled") setCategories(asArray(c.value.data));
    if (k.status === "fulfilled") setCountries(asArray(k.value.data));
    setLoaded(true);
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  const shopName = appearance?.name || settings.name || "goocommerce";

  const value = useMemo(
    () => ({
      settings,
      appearance,
      setAppearance,
      shopName,
      loaded,
      categories,
      countries,
      reload,
      money: (amount) => formatMoney(amount, settings),
      // Cart-side display: WooCommerce's split keeps totals excl. tax and the tax apart.
      cartIncl: settings.calcTaxes && settings.taxDisplayCart === "incl",
      shopIncl: settings.taxDisplayShop === "incl",
      countryName: (code) => countries.find((c) => c.code === code)?.name || code || "",
      stateName: (country, state) =>
        countries.find((c) => c.code === country)?.states?.find((s) => s.code === state)?.name || state || "",
    }),
    [settings, appearance, shopName, loaded, categories, countries, reload]
  );

  return <ShopContext.Provider value={value}>{children}</ShopContext.Provider>;
};

export const useShop = () => useContext(ShopContext);
