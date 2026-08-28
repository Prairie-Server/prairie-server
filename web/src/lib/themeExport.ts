import type { ThemeId } from "@/lib/themes";
import { THEME_IDS } from "@/lib/themes";
import type { ThemeToken } from "@/lib/themeTokens";
import { stringifyUnknown } from "@/lib/stringifyUnknown";

/** Portable theme file format for import/export and catalog distribution. */
export interface PrairieThemeFile {
  version: 1;
  name: string;
  description?: string;
  author?: string;
  baseTheme: ThemeId;
  vars: Partial<Record<ThemeToken, string>>;
  customCss: string;
  createdAt: string;
}

export const MAX_CSS_SIZE = 64 * 1024; // 64 KB

/** Parse a JSON string into a ThemeVarOverrides map. Returns empty object on failure. */
export function parseVarsJson(
  raw: string | null | undefined,
): Partial<Record<ThemeToken, string>> {
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw);
    return typeof parsed === "object" && parsed !== null ? parsed : {};
  } catch {
    return {};
  }
}

/** Serialize a theme config into the portable file format. */
export function serializeTheme(opts: {
  name: string;
  description?: string;
  author?: string;
  baseTheme: ThemeId;
  vars: Partial<Record<ThemeToken, string>>;
  customCss: string;
}): PrairieThemeFile {
  return {
    version: 1,
    name: opts.name,
    description: opts.description,
    author: opts.author,
    baseTheme: opts.baseTheme,
    vars: opts.vars,
    customCss: opts.customCss,
    createdAt: new Date().toISOString(),
  };
}

/** Parse and validate a theme file from raw JSON. Throws on invalid input. */
export function parseThemeFile(json: unknown): PrairieThemeFile {
  if (!json || typeof json !== "object") {
    throw new Error("Invalid theme file: expected a JSON object");
  }

  const obj = json as Record<string, unknown>;

  if (obj.version !== 1) {
    throw new Error(
      `Unsupported theme file version: ${stringifyUnknown(obj.version)}`,
    );
  }
  if (typeof obj.name !== "string" || !obj.name.trim()) {
    throw new Error("Theme file must have a name");
  }
  if (
    typeof obj.baseTheme !== "string" ||
    !(THEME_IDS as readonly string[]).includes(obj.baseTheme)
  ) {
    throw new Error(`Invalid baseTheme: ${stringifyUnknown(obj.baseTheme)}`);
  }
  if (obj.vars != null && typeof obj.vars !== "object") {
    throw new Error("vars must be an object");
  }

  const customCss = typeof obj.customCss === "string" ? obj.customCss : "";
  if (customCss.length > MAX_CSS_SIZE) {
    throw new Error(
      `Custom CSS exceeds maximum size of ${MAX_CSS_SIZE / 1024} KB`,
    );
  }

  return {
    version: 1,
    name: obj.name.trim(),
    description:
      typeof obj.description === "string" ? obj.description : undefined,
    author: typeof obj.author === "string" ? obj.author : undefined,
    baseTheme: obj.baseTheme as ThemeId,
    vars: (obj.vars ?? {}) as Partial<Record<ThemeToken, string>>,
    customCss,
    createdAt:
      typeof obj.createdAt === "string"
        ? obj.createdAt
        : new Date().toISOString(),
  };
}

/** Trigger a browser download of a theme file. */
export function downloadTheme(theme: PrairieThemeFile): void {
  const json = JSON.stringify(theme, null, 2);
  const blob = new Blob([json], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `${theme.name.replace(/[^a-zA-Z0-9_-]/g, "_")}.prairie-theme.json`;
  a.click();
  URL.revokeObjectURL(url);
}

/** Read a theme file from a File input. Returns the parsed theme. */
export async function readThemeFile(file: File): Promise<PrairieThemeFile> {
  const text = await file.text();
  let json: unknown;
  try {
    json = JSON.parse(text);
  } catch {
    throw new Error("File is not valid JSON");
  }
  return parseThemeFile(json);
}
