import { useEffect, useState } from "react";
import { api, asArray } from "../../api/client";
import { useShop } from "../../context/ShopContext";

let cache = null;

// useAllCountries returns every country for admin pickers: GET /v1/countries
// (all ISO countries) merged with the shop's /v1/shop/countries (which carry
// the states), falling back to the shop's selling countries alone.
export const useAllCountries = () => {
  const { countries: shopCountries } = useShop();
  const [all, setAll] = useState(cache);
  useEffect(() => {
    if (cache) return;
    api
      .get("/v1/countries")
      .then(({ data }) => {
        cache = asArray(data).map((c) => ({ code: c.code || c.iso, name: c.name, states: c.states || [] }));
        setAll(cache);
      })
      .catch(() => {});
  }, []);
  if (!all || all.length === 0) return shopCountries;
  return all.map((c) => {
    const s = shopCountries.find((x) => x.code === c.code);
    return s && s.states?.length ? { ...c, states: s.states } : c;
  });
};
