import { apiUrl } from "../api/client";

export const COLOR_KEYS = [
  ["primary", "--goo-primary"],
  ["primaryText", "--goo-primary-text"],
  ["secondary", "--goo-secondary"],
  ["accent", "--goo-accent"],
  ["background", "--goo-bg"],
  ["surface", "--goo-surface"],
  ["text", "--goo-text"],
  ["muted", "--goo-muted"],
  ["border", "--goo-border"],
  ["header", "--goo-header"],
  ["headerText", "--goo-header-text"],
  ["footer", "--goo-footer"],
  ["footerText", "--goo-footer-text"],
  ["sale", "--goo-sale"],
];

export const SYSTEM_FONTS = ["system-ui", "Arial", "Helvetica", "Georgia", "Times New Roman", "Verdana", "Courier New", "serif", "sans-serif", "monospace"];

export const GOOGLE_FONTS = [
  "Inter",
  "Roboto",
  "Open Sans",
  "Lato",
  "Montserrat",
  "Poppins",
  "Nunito",
  "Raleway",
  "Playfair Display",
  "Merriweather",
  "Lora",
  "Cormorant Garamond",
  "Dancing Script",
  "Josefin Sans",
];

const fontStack = (name, fallback) => (name ? `"${name}", ${fallback}` : fallback);

// cssVarsFor maps an appearance JSON onto the storefront CSS variables.
export const cssVarsFor = (app) => {
  const vars = {};
  if (!app) return vars;
  const colors = app.colors || {};
  COLOR_KEYS.forEach(([key, cssVar]) => {
    if (colors[key]) vars[cssVar] = colors[key];
  });
  if (app.fonts?.heading) vars["--goo-font-heading"] = fontStack(app.fonts.heading, "Georgia, serif");
  if (app.fonts?.body) vars["--goo-font-body"] = fontStack(app.fonts.body, "system-ui, sans-serif");
  if (app.radius !== undefined && app.radius !== null && app.radius !== "") vars["--goo-radius"] = `${Number(app.radius)}px`;
  return vars;
};

export const loadGoogleFonts = (fonts) => {
  const families = [...new Set([fonts?.heading, fonts?.body].filter(Boolean))].filter((f) => !SYSTEM_FONTS.includes(f));
  const id = "goo-google-fonts";
  let link = document.getElementById(id);
  if (families.length === 0) {
    link?.remove();
    return;
  }
  const href = `https://fonts.googleapis.com/css2?${families
    .map((f) => `family=${encodeURIComponent(f).replace(/%20/g, "+")}:wght@400;600;700`)
    .join("&")}&display=swap`;
  if (!link) {
    link = document.createElement("link");
    link.id = id;
    link.rel = "stylesheet";
    document.head.appendChild(link);
  }
  if (link.href !== href) link.href = href;
};

const setFavicon = (href) => {
  if (!href) return;
  let link = document.querySelector('link[rel="icon"]');
  if (!link) {
    link = document.createElement("link");
    link.rel = "icon";
    document.head.appendChild(link);
  }
  link.removeAttribute("type");
  link.href = href;
};

const injectCustomCss = (version) => {
  const id = "goo-custom-css";
  let link = document.getElementById(id);
  const href = `${apiUrl()}/v1/shop/custom.css${version ? `?v=${encodeURIComponent(version)}` : ""}`;
  if (!link) {
    link = document.createElement("link");
    link.id = id;
    link.rel = "stylesheet";
    document.head.appendChild(link);
  }
  if (link.href !== href) link.href = href;
};

// applyAppearance applies the shop look globally: CSS variables on :root (so it
// works before custom.css loads), Google Fonts, favicon and the API's custom.css.
export const applyAppearance = (app) => {
  const root = document.documentElement;
  Object.entries(cssVarsFor(app)).forEach(([k, v]) => root.style.setProperty(k, v));
  loadGoogleFonts(app?.fonts);
  setFavicon(app?.favicon || app?.logo?.src);
  injectCustomCss(app?.updatedAt);
};
