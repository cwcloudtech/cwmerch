import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { toast } from "react-toastify";
import { FiDownload } from "react-icons/fi";
import { absoluteUrl, downloadFile } from "../api/client";
import { useI18n } from "../i18n/I18nContext";

const isDownloadHref = (href) => typeof href === "string" && href.includes("/ai-downloads/");

// Markdown renders an assistant answer (GFM: tables, lists, code). Links to
// files the assistant generated (/v1/ai-downloads/{id}) are short-lived and
// rendered as a download chip that fetches with the caller's auth; once the
// message is older than the download TTL they show as expired.
const Markdown = ({ children, downloadExpiresAt = null }) => {
  const { t, errMsg } = useI18n();

  const renderAnchor = ({ href, children: linkChildren }) => {
    if (isDownloadHref(href)) {
      const expired = downloadExpiresAt != null && Date.now() >= downloadExpiresAt;
      if (expired) {
        return (
          <span className="md-download-expired" title={t("ai.downloadExpiredHint")}>
            {linkChildren} ({t("ai.expired")})
          </span>
        );
      }
      const url = absoluteUrl(href);
      const onClick = async (e) => {
        e.preventDefault();
        try {
          await downloadFile(url, { fallbackName: "download.pdf" });
        } catch (err) {
          if (!err?.response) window.open(url, "_blank", "noopener");
          else toast.error(errMsg(err));
        }
      };
      return (
        <a href={url} onClick={onClick} className="md-download" title={t("ai.downloadTemporaryHint")}>
          <FiDownload aria-hidden="true" /> {linkChildren}
        </a>
      );
    }
    return (
      <a href={href} target="_blank" rel="noopener noreferrer">
        {linkChildren}
      </a>
    );
  };

  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={{ a: renderAnchor }}>
        {children || ""}
      </ReactMarkdown>
    </div>
  );
};

export default Markdown;
