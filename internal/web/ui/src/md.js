import { marked } from "marked";
import DOMPurify from "dompurify";

marked.setOptions({ breaks: true, gfm: true });

/** Parse markdown → sanitized HTML for {@html}. */
export function renderMd(src) {
  if (!src) return "";
  const html = marked.parse(String(src), { async: false });
  return DOMPurify.sanitize(html, {
    USE_PROFILES: { html: true },
  });
}
