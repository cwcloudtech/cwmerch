import { useCallback, useEffect, useRef, useState } from "react";
import { usePermalinks } from "../lib/permalinks";
import { Link } from "react-router-dom";
import { FiChevronLeft, FiChevronRight } from "react-icons/fi";
import { useI18n } from "../i18n/I18nContext";

const prefersReducedMotion = () =>
  typeof window !== "undefined" && window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

const CtaLink = ({ href, children }) => {
  const links = usePermalinks();
  return /^https?:\/\//i.test(href) ? (
    <a className="btn btn-primary btn-lg" href={href}>
      {children}
    </a>
  ) : (
    <Link className="btn btn-primary btn-lg" to={href || links.shop}>
      {children}
    </Link>
  );
};

// HeroSlider is the home banner: autoplay (paused on hover/focus and when the
// user prefers reduced motion), arrows, dots and touch swipe.
const HeroSlider = ({ banner, preview = false }) => {
  const { t, dir } = useI18n();
  const slides = (banner?.slides || []).filter((s) => s.image || s.title);
  const [index, setIndex] = useState(0);
  const [paused, setPaused] = useState(false);
  const touchX = useRef(null);
  const count = slides.length;
  const current = count ? index % count : 0;

  const go = useCallback((i) => setIndex(((i % count) + count) % count), [count]);

  useEffect(() => {
    if (!banner?.autoplay || count < 2 || paused || prefersReducedMotion()) return undefined;
    const ms = Math.max(2, Number(banner.intervalSeconds) || 6) * 1000;
    const id = setInterval(() => setIndex((i) => (i + 1) % count), ms);
    return () => clearInterval(id);
  }, [banner?.autoplay, banner?.intervalSeconds, count, paused]);

  if (!banner?.enabled || count === 0) return null;

  const onTouchStart = (e) => {
    touchX.current = e.touches[0].clientX;
  };
  const onTouchEnd = (e) => {
    if (touchX.current === null) return;
    const dx = e.changedTouches[0].clientX - touchX.current;
    // Swiping towards the reading start goes forward (leftward in LTR, rightward in RTL).
    const forward = dir === "rtl" ? dx > 0 : dx < 0;
    if (Math.abs(dx) > 40) go(current + (forward ? 1 : -1));
    touchX.current = null;
  };

  return (
    <section
      className={`hero hero-${banner.height || "medium"} ${preview ? "hero-preview" : ""}`}
      aria-roledescription="carousel"
      aria-label={t("home.banner")}
      onMouseEnter={() => setPaused(true)}
      onMouseLeave={() => setPaused(false)}
      onFocus={() => setPaused(true)}
      onBlur={() => setPaused(false)}
      onTouchStart={onTouchStart}
      onTouchEnd={onTouchEnd}
    >
      {slides.map((s, i) => (
        <div
          key={i}
          className={`hero-slide align-${s.align || "center"} ${i === current ? "is-active" : ""}`}
          role="group"
          aria-roledescription="slide"
          aria-label={t("home.slideOf", { n: i + 1, count })}
          aria-hidden={i !== current}
        >
          {s.image && (
            <picture>
              {s.mobileImage && <source media="(max-width: 640px)" srcSet={s.mobileImage} />}
              <img src={s.image} alt={s.title || ""} loading={i === 0 ? "eager" : "lazy"} />
            </picture>
          )}
          <div className="hero-overlay" style={{ background: `rgba(0,0,0,${Math.min(90, Math.max(0, Number(s.overlay) || 0)) / 100})` }} />
          <div className="hero-content container" style={{ color: s.textColor || "#fff" }}>
            {s.title && <p className="hero-title" dir="auto">{s.title}</p>}
            {s.subtitle && <p className="hero-subtitle" dir="auto">{s.subtitle}</p>}
            {s.buttonText && i === current && <CtaLink href={s.buttonLink}>{s.buttonText}</CtaLink>}
          </div>
        </div>
      ))}
      {count > 1 && (
        <>
          <button type="button" className="hero-arrow hero-prev" onClick={() => go(current - 1)} aria-label={t("home.previousSlide")}>
            <FiChevronLeft className="flip-rtl" />
          </button>
          <button type="button" className="hero-arrow hero-next" onClick={() => go(current + 1)} aria-label={t("home.nextSlide")}>
            <FiChevronRight className="flip-rtl" />
          </button>
          <div className="hero-dots">
            {slides.map((_, i) => (
              <button
                key={i}
                type="button"
                className={`hero-dot ${i === current ? "is-active" : ""}`}
                aria-label={t("home.goToSlide", { n: i + 1 })}
                aria-current={i === current}
                onClick={() => go(i)}
              />
            ))}
          </div>
        </>
      )}
    </section>
  );
};

export default HeroSlider;
