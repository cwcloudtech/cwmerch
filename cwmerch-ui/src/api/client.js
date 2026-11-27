import axios from "axios";

// The API base URL is runtime config: /config.js (written by the container
// entrypoint from ${CWMERCH_API_URL}) sets window.__CWMERCH__.
export const apiUrl = () => {
  const configured = typeof window !== "undefined" && window.__CWMERCH__ && window.__CWMERCH__.apiUrl;
  return String(configured || "http://localhost:8080").replace(/\/+$/, "");
};

export const TOKEN_KEY = "cwmerch.token";
export const UNAUTHORIZED_EVENT = "cwmerch:unauthorized";
// Fired when a storefront call answers 403 errors.shopDisabled (the shop is closed).
export const SHOP_DISABLED_EVENT = "cwmerch:shop-disabled";

export const getToken = () => {
  try {
    return localStorage.getItem(TOKEN_KEY) || "";
  } catch {
    return "";
  }
};

export const api = axios.create({ headers: { "Content-Type": "application/json" } });

api.interceptors.request.use((config) => {
  config.baseURL = apiUrl();
  const token = getToken();
  if (token && !config.headers.Authorization) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

api.interceptors.response.use(
  (response) => response,
  (error) => {
    // A 401 on an authenticated call means the session is gone: clear it.
    const sentAuth = error?.config?.headers?.Authorization;
    if (error?.response?.status === 401 && sentAuth) {
      window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
    }
    if (error?.response?.status === 403 && error?.response?.data?.i18n_code === "errors.shopDisabled") {
      window.dispatchEvent(new Event(SHOP_DISABLED_EVENT));
    }
    return Promise.reject(error);
  }
);

// absoluteUrl turns an API-relative path (e.g. /v1/ai-downloads/x) into a full URL.
export const absoluteUrl = (href) => {
  if (!href) return href;
  if (/^https?:\/\//i.test(href) || href.startsWith("data:")) return href;
  return `${apiUrl()}${href.startsWith("/") ? "" : "/"}${href}`;
};

// normalizePaged accepts the documented paged shape {items,total,page,perPage,totalPages}
// and tolerates a bare array or the legacy {results}/{entries} shapes.
export const normalizePaged = (data, page = 1, perPage = 20) => {
  if (Array.isArray(data)) {
    return { items: data, total: data.length, page: 1, perPage: data.length || perPage, totalPages: 1 };
  }
  const items = (data && (data.items || data.results || data.entries)) || [];
  const total = Number(data?.total ?? data?.totalResults ?? items.length);
  const pp = Number(data?.perPage || perPage);
  let totalPages = Number(data?.totalPages || 0);
  if (!totalPages) totalPages = data?.hasNext || data?.hasMore ? Number(data?.page ?? page) + 1 : Math.max(1, Math.ceil(total / pp));
  return { items, total, page: Number(data?.page || page), perPage: pp, totalPages };
};

export const asArray = (data, key) => {
  if (Array.isArray(data)) return data;
  if (data && key && Array.isArray(data[key])) return data[key];
  if (data && Array.isArray(data.items)) return data.items;
  return [];
};

const filenameFromDisposition = (header) => {
  if (!header) return "";
  const star = /filename\*=(?:UTF-8'')?([^;]+)/i.exec(header);
  if (star) {
    try {
      return decodeURIComponent(star[1].trim().replace(/^"|"$/g, ""));
    } catch {
      /* fall through */
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(header);
  return plain ? plain[1].trim() : "";
};

// decodeBlobError turns a blob-typed error body back into the JSON {message, i18n_code}.
const decodeBlobError = async (error) => {
  const data = error?.response?.data;
  if (data instanceof Blob) {
    try {
      error.response.data = JSON.parse(await data.text());
    } catch {
      /* keep the blob */
    }
  }
  return error;
};

export const saveBlob = (blob, name) => {
  const blobUrl = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = blobUrl;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(blobUrl), 2000);
};

// fetchBlob gets a binary response with the caller's auth → {blob, name}
// (name from Content-Disposition, else the fallback).
export const fetchBlob = async (url, { params, fallbackName = "download" } = {}) => {
  try {
    const response = await api.get(url, { params, responseType: "blob" });
    return { blob: response.data, name: filenameFromDisposition(response.headers["content-disposition"]) || fallbackName };
  } catch (error) {
    throw await decodeBlobError(error);
  }
};

// openBlobInTab requests a binary response (e.g. a PDF preview) and opens it in a
// new tab via a blob URL. The tab is opened synchronously (before the request)
// so popup blockers keep it; it is closed again if the request fails.
export const openBlobInTab = async (method, url, { data, params, fallbackName = "document.pdf" } = {}) => {
  const tab = window.open("", "_blank");
  let response;
  try {
    response = await api.request({ method, url, data, params, responseType: "blob" });
  } catch (error) {
    tab?.close();
    throw await decodeBlobError(error);
  }
  const blobUrl = URL.createObjectURL(response.data);
  if (tab) tab.location.href = blobUrl;
  else saveBlob(response.data, filenameFromDisposition(response.headers["content-disposition"]) || fallbackName);
  setTimeout(() => URL.revokeObjectURL(blobUrl), 60000);
};

// downloadFile fetches a binary response with the caller's auth and saves it
// under the Content-Disposition filename (fallback: the given name).
export const downloadFile = async (url, { params, fallbackName = "download" } = {}) => {
  let response;
  try {
    response = await api.get(url, { params, responseType: "blob" });
  } catch (error) {
    throw await decodeBlobError(error);
  }
  const name = filenameFromDisposition(response.headers["content-disposition"]) || fallbackName;
  saveBlob(response.data, name);
  return name;
};
