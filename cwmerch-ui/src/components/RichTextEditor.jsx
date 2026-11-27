import { useEffect, useId, useMemo, useRef, useState } from "react";
import ReactQuill from "react-quill";
import "react-quill/dist/quill.snow.css";
import { toast } from "react-toastify";
import { FiCode, FiEdit3 } from "react-icons/fi";
import { api } from "../api/client";
import { uploadMedia } from "./admin";
import { useI18n } from "../i18n/I18nContext";

// HTML Quill can't represent: editing it visually would drop it (layout
// blocks, classes/styles, tables, embeds, WordPress shortcodes...).
const RICH_HTML = /<(div|span|table|thead|tbody|tr|td|th|iframe|video|audio|figure|figcaption|section|article|style|script|form|input|button|hr)\b|\sclass=|\sstyle=|\[[a-z_][\w-]*(?:\s[^\]]*)?\]/i;

export const needsHtmlMode = (html) => RICH_HTML.test(html || "");

// Quill's empty document.
const isEmptyQuill = (html) => !html || /^<p>(<br>)?<\/p>$/.test(html.trim());

// RichTextEditor edits HTML like the comwork-cloud-ui email editor (Quill),
// with an "HTML" tab to edit the source directly. Only the user's own edits are
// propagated: loading a value never rewrites it, and HTML Quill would mangle
// opens in the HTML tab.
const RichTextEditor = ({ label, hint, value, onChange, minHeight = 220 }) => {
  const { t, errMsg } = useI18n();
  const id = useId();
  const quillRef = useRef(null);
  const [mode, setMode] = useState(() => (needsHtmlMode(value) ? "html" : "visual"));
  const rich = needsHtmlMode(value);

  // A product loaded after mount with HTML Quill can't keep: switch to HTML.
  const lastRich = useRef(rich);
  useEffect(() => {
    if (rich && !lastRich.current) setMode("html");
    lastRich.current = rich;
  }, [rich]);

  const modules = useMemo(
    () => ({
      toolbar: {
        container: [
          [{ header: [2, 3, 4, false] }],
          ["bold", "italic", "underline", "strike"],
          ["link", "image"],
          [{ list: "ordered" }, { list: "bullet" }],
          ["blockquote", "code-block"],
          [{ color: [] }, { background: [] }],
          [{ align: [] }],
          ["clean"],
        ],
        handlers: {
          // Images go to the media library (not base64 in the description).
          image() {
            const input = document.createElement("input");
            input.type = "file";
            input.accept = "image/*";
            input.onchange = async () => {
              const file = input.files?.[0];
              if (!file) return;
              try {
                const { src } = await uploadMedia(api, file);
                const editor = quillRef.current?.getEditor();
                if (!editor) return;
                const range = editor.getSelection(true);
                editor.insertEmbed(range ? range.index : editor.getLength(), "image", src, "user");
              } catch (err) {
                toast.error(errMsg(err));
              }
            };
            input.click();
          },
        },
      },
      clipboard: { matchVisual: false },
    }),
    [errMsg]
  );

  return (
    <div className="field rich-editor">
      <div className="rich-editor-head">
        <label className="field-label" htmlFor={mode === "html" ? id : undefined}>
          {label}
        </label>
        <div className="rich-editor-tabs" role="tablist" aria-label={label}>
          <button type="button" role="tab" aria-selected={mode === "visual"} className={mode === "visual" ? "is-active" : ""} onClick={() => setMode("visual")}>
            <FiEdit3 aria-hidden="true" /> {t("editor.visual")}
          </button>
          <button type="button" role="tab" aria-selected={mode === "html"} className={mode === "html" ? "is-active" : ""} onClick={() => setMode("html")}>
            <FiCode aria-hidden="true" /> {t("editor.html")}
          </button>
        </div>
      </div>
      {mode === "visual" && rich && <p className="notice notice-warning small">{t("editor.richWarning")}</p>}
      {mode === "visual" ? (
        <div className="rich-editor-quill" style={{ "--rich-min-height": `${minHeight}px` }}>
          <ReactQuill
            ref={quillRef}
            theme="snow"
            value={value || ""}
            modules={modules}
            onChange={(html, _delta, source) => {
              if (source !== "user") return;
              onChange(isEmptyQuill(html) ? "" : html);
            }}
          />
        </div>
      ) : (
        <textarea id={id} className="input mono-area rich-editor-source" value={value || ""} onChange={(e) => onChange(e.target.value)} rows={Math.max(6, Math.round(minHeight / 22))} spellCheck={false} />
      )}
      {hint && <p className="hint">{hint}</p>}
    </div>
  );
};

export default RichTextEditor;
