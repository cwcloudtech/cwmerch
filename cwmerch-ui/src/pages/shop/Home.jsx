import { Link } from "react-router-dom";
import { usePermalinks } from "../../lib/permalinks";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { api, normalizePaged } from "../../api/client";
import { useAsync } from "../../lib/useAsync";
import { useSeo } from "../../lib/seo";
import { useShop } from "../../context/ShopContext";
import { useI18n } from "../../i18n/I18nContext";
import HeroSlider from "../../components/HeroSlider";
import { ProductGrid } from "../../components/ProductCard";
import { Spinner } from "../../components/ui";

const DEFAULT_SECTIONS = [
  { type: "categories", limit: 8 },
  { type: "featured", limit: 8 },
  { type: "latest", limit: 8 },
];

const QUERY = {
  featured: { featured: true },
  latest: { orderby: "date" },
  onSale: { onSale: true },
  bestSellers: { orderby: "popularity" },
};

const ProductsSection = ({ section }) => {
  const links = usePermalinks();
  const { t } = useI18n();
  const limit = Math.min(100, Number(section.limit) || 8);
  const { data, loading } = useAsync(
    async () => normalizePaged((await api.get("/v1/shop/products", { params: { ...QUERY[section.type], perPage: limit, page: 1 } })).data).items,
    [section.type, limit],
    []
  );
  if (!loading && data.length === 0) return null;
  const more = links.shop + (section.type === "onSale" ? "?onSale=1" : section.type === "latest" ? "?orderby=date" : section.type === "bestSellers" ? "?orderby=popularity" : "");
  return (
    <section className="home-section container">
      <div className="section-head">
        <h2>{section.title || t(`home.section.${section.type}`)}</h2>
        <Link to={more} className="link-more">
          {t("home.viewAll")}
        </Link>
      </div>
      {loading ? <Spinner /> : <ProductGrid products={data} />}
    </section>
  );
};

export const CategoryGrid = ({ categories }) => {
  const links = usePermalinks();
  return (
  <div className="category-grid">
    {categories.map((c) => (
      <Link key={c.id} to={links.category(c)} className="category-card">
        {c.image?.src ? <img src={c.image.src} alt={c.name} loading="lazy" /> : <div className="img-placeholder" aria-hidden="true" />}
        <span className="category-card-name">
          {c.name}
          {Number(c.count) > 0 && <small> ({c.count})</small>}
        </span>
      </Link>
    ))}
  </div>
  );
};

const CategoriesSection = ({ section }) => {
  const { categories } = useShop();
  const { t } = useI18n();
  const top = categories.filter((c) => !Number(c.parent) && c.slug !== "uncategorized").slice(0, Number(section.limit) || 8);
  if (top.length === 0) return null;
  return (
    <section className="home-section container">
      <div className="section-head">
        <h2>{section.title || t("home.section.categories")}</h2>
      </div>
      <CategoryGrid categories={top} />
    </section>
  );
};

const TextSection = ({ section }) => (
  <section className="home-section container text-section">
    {section.title && <h2>{section.title}</h2>}
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{section.content || ""}</ReactMarkdown>
    </div>
  </section>
);

const Home = () => {
  const { appearance, shopName, settings } = useShop();
  const origin = typeof window !== "undefined" ? window.location.origin : "";
  useSeo({
    title: appearance?.tagline ? `${shopName} – ${appearance.tagline}` : shopName,
    description: appearance?.tagline || settings.description,
    image: appearance?.banner?.slides?.[0]?.image || appearance?.logo?.src,
    jsonLd: [
      { "@context": "https://schema.org", "@type": "WebSite", name: shopName, url: origin, potentialAction: { "@type": "SearchAction", target: `${origin}/shop?search={search_term_string}`, "query-input": "required name=search_term_string" } },
      { "@context": "https://schema.org", "@type": "Organization", name: shopName, url: origin, ...(appearance?.logo?.src ? { logo: appearance.logo.src } : {}) },
    ],
  });
  const sections = appearance?.homeSections?.length ? appearance.homeSections : DEFAULT_SECTIONS;
  return (
    <div className="home">
      <h1 className="sr-only">{shopName}</h1>
      <HeroSlider banner={appearance?.banner} />
      {sections.map((s, i) => {
        if (s.type === "categories") return <CategoriesSection key={i} section={s} />;
        if (s.type === "text") return <TextSection key={i} section={s} />;
        if (QUERY[s.type]) return <ProductsSection key={i} section={s} />;
        return null;
      })}
    </div>
  );
};

export default Home;
