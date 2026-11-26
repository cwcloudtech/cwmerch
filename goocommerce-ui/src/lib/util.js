export const slugify = (s) =>
  String(s || "")
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");

export const sameOption = (a, b) => slugify(a) === slugify(b);

export const idsToText = (ids) => (Array.isArray(ids) ? ids.join(", ") : "");
export const textToIds = (text) =>
  String(text || "")
    .split(/[\s,;]+/)
    .map((x) => parseInt(x, 10))
    .filter((n) => Number.isFinite(n) && n > 0);

export const listToText = (list, sep = ", ") => (Array.isArray(list) ? list.join(sep) : "");
export const textToList = (text, re = /[,;\n]+/) =>
  String(text || "")
    .split(re)
    .map((x) => x.trim())
    .filter(Boolean);

export const numOrNull = (v) => (v === "" || v === null || v === undefined ? null : Number(v));

export const isoDay = (d) => {
  const x = d instanceof Date ? d : new Date(d);
  const pad = (n) => String(n).padStart(2, "0");
  return `${x.getFullYear()}-${pad(x.getMonth() + 1)}-${pad(x.getDate())}`;
};

export const daysAgo = (n) => {
  const d = new Date();
  d.setDate(d.getDate() - n);
  return isoDay(d);
};

export const dateInput = (v) => (v ? String(v).slice(0, 10) : "");
export const dateToApi = (v) => (v ? `${v}T00:00:00Z` : null);

export const ORDER_STATUSES = ["pending", "processing", "on-hold", "completed", "cancelled", "refunded", "failed", "checkout-draft"];
export const ROLES = ["superuser", "manager", "customer", "disabled", "ban"];
export const isAdminRole = (role) => role === "superuser" || role === "manager";

// stripHtml for short previews of HTML product descriptions.
export const stripHtml = (html) => String(html || "").replace(/<[^>]*>/g, " ").replace(/\s+/g, " ").trim();
