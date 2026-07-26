/**
 * CSS sanitizer that strips external resource loading from raw CSS.
 *
 * Blocks: @import, external url(), external @font-face src
 * Allows: url(data:...), url(/...), url(#...), all other CSS
 *
 * Comments are stripped and CSS escapes normalized before matching so
 * comment-obfuscated @import and escaped external url() forms cannot bypass checks.
 */

/** Matches @import rules (with url() or bare string) */
const AT_IMPORT_RE = /@import\s+(?:url\(.*?\)|['"].*?['"])[^;]*;?/gi;

function stripCssComments(css: string): string {
  let out = "";
  for (let i = 0; i < css.length; ) {
    if (css[i] === "/" && css[i + 1] === "*") {
      const end = css.indexOf("*/", i + 2);
      if (end < 0) break;
      i = end + 2;
      continue;
    }
    out += css[i];
    i += 1;
  }
  return out;
}

function unescapeCss(value: string): string {
  let out = "";
  for (let i = 0; i < value.length; ) {
    if (value[i] !== "\\" || i + 1 >= value.length) {
      out += value[i];
      i += 1;
      continue;
    }
    let j = i + 1;
    let hexDigits = 0;
    while (j < value.length && hexDigits < 6 && /[0-9a-fA-F]/.test(value[j]!)) {
      j += 1;
      hexDigits += 1;
    }
    if (hexDigits > 0) {
      const code = Number.parseInt(value.slice(i + 1, j), 16);
      if (Number.isFinite(code)) {
        // CSS allows escapes beyond Unicode; fromCodePoint throws above U+10FFFF.
        out += code >= 0 && code <= 0x10ffff
          ? String.fromCodePoint(code)
          : "\uFFFD";
      }
      if (j < value.length && /[ \t\n\r\f]/.test(value[j]!)) j += 1;
      i = j;
      continue;
    }
    out += value[i + 1];
    i += 2;
  }
  return out;
}

/** Check if a single url() value is safe (data:, local path, or fragment) */
function isSafeUrl(urlContent: string): boolean {
  const trimmed = unescapeCss(urlContent).trim().replace(/^['"]|['"]$/g, "");
  if (!trimmed) return true;
  if (trimmed.startsWith("data:")) return true;
  if (trimmed.startsWith("/") && !trimmed.startsWith("//")) return true;
  if (trimmed.startsWith("#")) return true;
  // Relative paths (no scheme) are fine — they resolve to the same origin
  if (!/^[a-z][a-z0-9+.-]*:/i.test(trimmed) && !trimmed.startsWith("//")) return true;
  return false;
}

/**
 * Sanitize raw CSS by stripping external resource references.
 * Returns the cleaned CSS string.
 */
export function sanitizeCss(css: string): string {
  let result = stripCssComments(css);

  // Strip @import rules entirely
  result = result.replace(AT_IMPORT_RE, "/* [blocked @import] */");

  // Replace external url() with empty url()
  result = result.replace(
    /url\(\s*(['"]?)([\s\S]*?)\1\s*\)/gi,
    (_match, _quote, content: string) => {
      if (isSafeUrl(content)) return _match;
      return "/* [blocked external url] */";
    },
  );

  return result;
}

/** Quick check if CSS contains patterns that would be sanitized. */
export function hasUnsafeCss(css: string): boolean {
  return css !== sanitizeCss(css);
}
