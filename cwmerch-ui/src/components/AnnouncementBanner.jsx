import { useState } from "react";
import { FiX } from "react-icons/fi";
import { useI18n } from "../i18n/I18nContext";

const dismissKey = (a) => `cwmerch.announcement.${a.id}`;

const isDismissed = (a) => {
  try {
    return window.localStorage.getItem(dismissKey(a)) === String(a.updatedAt || "");
  } catch {
    return false;
  }
};

// AnnouncementBanner is one announcement: modern translucent card tinted by
// the theme's primary colour (or the announcement's own background).
export const AnnouncementBanner = ({ announcement: a, onDismiss, preview }) => {
  const { t } = useI18n();
  const style = a.background ? { "--announcement-tint": a.background } : undefined;
  return (
    <section className="announcement-card" style={style} aria-label={t("announcements.label")}>
      <div className="announcement-content rich-content" dangerouslySetInnerHTML={{ __html: a.content }} />
      {a.dismissible && (
        <button type="button" className="announcement-close" onClick={preview ? undefined : onDismiss} aria-label={t("announcements.dismiss")} title={t("announcements.dismiss")}>
          <FiX aria-hidden="true" />
        </button>
      )}
    </section>
  );
};

// Announcements shows the active announcements for the current page.
const Announcements = ({ items, home }) => {
  const [hidden, setHidden] = useState([]);
  const now = Date.now();
  const visible = (items || []).filter(
    (a) => (a.placement === "all" || home) && (!a.expiresAt || new Date(a.expiresAt).getTime() > now) && !hidden.includes(a.id) && !isDismissed(a)
  );
  if (visible.length === 0) return null;
  return (
    <div className="container announcements">
      {visible.map((a) => (
        <AnnouncementBanner
          key={a.id}
          announcement={a}
          onDismiss={() => {
            try {
              window.localStorage.setItem(dismissKey(a), String(a.updatedAt || ""));
            } catch {
              /* storage unavailable: hidden until the next page load */
            }
            setHidden((h) => [...h, a.id]);
          }}
        />
      ))}
    </div>
  );
};

export default Announcements;
