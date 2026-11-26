import { useEffect } from "react";
import { stripHtml } from "./util";

const setMeta = (attr, key, content) => {
  let el = document.head.querySelector(`meta[${attr}="${key}"]`);
  if (content === undefined || content === null || content === "") {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("meta");
    el.setAttribute(attr, key);
    document.head.appendChild(el);
  }
  el.setAttribute("content", content);
};

const setLink = (rel, href) => {
  let el = document.head.querySelector(`link[rel="${rel}"]`);
  if (!href) {
    el?.remove();
    return;
  }
  if (!el) {
    el = document.createElement("link");
    el.rel = rel;
    document.head.appendChild(el);
  }
  el.href = href;
};

export const truncate = (text, max = 160) => {
  const s = stripHtml(text);
  return s.length > max ? `${s.slice(0, max - 1).trimEnd()}…` : s;
};

// useSeo updates the document head on navigation (the API pre-renders the same
// tags for crawlers via /v1/seo/html; this keeps them right for in-app navigation).
export const useSeo = ({ title, description, image, canonical, jsonLd, noindex, type = "website" } = {}) => {
  const json = jsonLd ? JSON.stringify(jsonLd) : "";
  useEffect(() => {
    if (title) document.title = title;
    const url = canonical || (typeof window !== "undefined" ? window.location.origin + window.location.pathname : "");
    const desc = description ? truncate(description) : "";
    setMeta("name", "description", desc);
    setMeta("name", "robots", noindex ? "noindex, nofollow" : "index, follow");
    setLink("canonical", noindex ? "" : url);
    setMeta("property", "og:type", type);
    setMeta("property", "og:title", title);
    setMeta("property", "og:description", desc);
    setMeta("property", "og:url", url);
    setMeta("property", "og:image", image);
    setMeta("name", "twitter:card", image ? "summary_large_image" : "summary");
    setMeta("name", "twitter:title", title);
    setMeta("name", "twitter:description", desc);
    setMeta("name", "twitter:image", image);
    let script = document.getElementById("goo-jsonld");
    if (json) {
      if (!script) {
        script = document.createElement("script");
        script.type = "application/ld+json";
        script.id = "goo-jsonld";
        document.head.appendChild(script);
      }
      script.textContent = json;
    } else {
      script?.remove();
    }
  }, [title, description, image, canonical, json, noindex, type]);
};
