import { useEffect, useState } from "react";
import { stripHtml } from "./util";
import { api } from "../api/client";

// The server-resolved heads (SEO title/description templates, robots,
// social cards), per path, for the page's lifetime.
const heads = new Map();

const useServerHead = (enabled, path) => {
  const [head, setHead] = useState(() => (enabled ? heads.get(path) : null));
  useEffect(() => {
    if (!enabled) {
      setHead(null);
      return undefined;
    }
    if (heads.has(path)) {
      setHead(heads.get(path));
      return undefined;
    }
    let live = true;
    api
      .get("/v1/seo/head", { params: { path } })
      .then(({ data }) => {
        heads.set(path, data);
        if (live) setHead(data);
      })
      .catch(() => {
        if (live) setHead(null);
      });
    return () => {
      live = false;
    };
  }, [enabled, path]);
  return head;
};

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
// server: the page's SEO settings (title/description templates, robots,
// social cards) win, read from /v1/seo/head (the same as the crawlers get).
export const useSeo = ({ title, description, image, canonical, jsonLd, noindex, type = "website", server = false } = {}) => {
  const json = jsonLd ? JSON.stringify(jsonLd) : "";
  const path = typeof window !== "undefined" ? window.location.pathname : "/";
  const head = useServerHead(server && !noindex, path);
  if (head) {
    title = head.title || title;
    description = head.description || description;
    noindex = head.noIndex;
    canonical = head.canonical || canonical;
  }
  const robots = head?.robots || (noindex ? "noindex, nofollow" : "index, follow");
  const og = head ? { on: head.openGraph, title: head.ogTitle, description: head.ogDescription, image: head.ogImage || image } : null;
  const tw = head ? { on: head.twitter, title: head.twitterTitle, description: head.twitterDescription, image: head.twitterImage || image, site: head.twitterSite } : null;
  const social = JSON.stringify([og, tw]);
  useEffect(() => {
    if (title) document.title = title;
    const url = canonical || (typeof window !== "undefined" ? window.location.origin + window.location.pathname : "");
    const desc = description ? truncate(description) : "";
    const [o, x] = JSON.parse(social);
    const ogc = o || { on: true, title, description: desc, image };
    const twc = x || { on: true, title, description: desc, image };
    setMeta("name", "description", desc);
    setMeta("name", "robots", robots);
    setLink("canonical", noindex ? "" : url);
    setMeta("property", "og:type", ogc.on ? type : "");
    setMeta("property", "og:title", ogc.on ? ogc.title : "");
    setMeta("property", "og:description", ogc.on ? truncate(ogc.description || "") : "");
    setMeta("property", "og:url", ogc.on ? url : "");
    setMeta("property", "og:image", ogc.on ? ogc.image : "");
    setMeta("name", "twitter:card", twc.on ? (twc.image ? "summary_large_image" : "summary") : "");
    setMeta("name", "twitter:title", twc.on ? twc.title : "");
    setMeta("name", "twitter:description", twc.on ? truncate(twc.description || "") : "");
    setMeta("name", "twitter:image", twc.on ? twc.image : "");
    setMeta("name", "twitter:site", twc.on && twc.site ? `@${twc.site}` : "");
    let script = document.getElementById("cwmerch-jsonld");
    if (json) {
      if (!script) {
        script = document.createElement("script");
        script.type = "application/ld+json";
        script.id = "cwmerch-jsonld";
        document.head.appendChild(script);
      }
      script.textContent = json;
    } else {
      script?.remove();
    }
  }, [title, description, image, canonical, json, noindex, type, robots, social]);
};
