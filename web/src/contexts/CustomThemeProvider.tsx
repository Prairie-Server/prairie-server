import { createContext, useContext, useEffect, useRef } from "react";
import type { ReactNode } from "react";
import { useCustomTheme } from "@/hooks/useCustomTheme";
import { useAdminPublicCss } from "@/hooks/queries/theme";
import type { ThemeVarOverrides } from "@/hooks/useCustomTheme";

interface CustomThemeContextValue {
  /** User variable overrides (live state). */
  vars: ThemeVarOverrides;
  /** User raw CSS (live state). */
  customCss: string;
  /** Admin variable overrides. */
  adminVars: Record<string, string>;
  /** Admin raw CSS. */
  adminRawCss: string;
  /** Whether the custom theme system has any active overrides. */
  hasOverrides: boolean;
}

const CustomThemeContext = createContext<CustomThemeContextValue | null>(null);

/** Reject values that could break out of CSS declarations. */
const UNSAFE_VALUE = /[;{}]|\/\*|<\//;

function buildVarOverrideCSS(vars: Record<string, string>): string {
  const entries = Object.entries(vars).filter(([, v]) => v !== "" && !UNSAFE_VALUE.test(v));
  if (entries.length === 0) return "";
  const props = entries.map(([k, v]) => `  --${k}: ${v};`).join("\n");
  return `:root {\n${props}\n}`;
}

function getOrCreateStyle(id: string): HTMLStyleElement {
  let el = document.getElementById(id) as HTMLStyleElement | null;
  if (!el) {
    el = document.createElement("style");
    el.id = id;
    document.head.appendChild(el);
  }
  return el;
}

export function CustomThemeProvider({ children }: { children: ReactNode }) {
  const { vars, customCss } = useCustomTheme();
  const { data: adminCss } = useAdminPublicCss();

  const adminVarsRef = useRef<HTMLStyleElement>(null);
  const adminRawRef = useRef<HTMLStyleElement>(null);
  const userVarsRef = useRef<HTMLStyleElement>(null);
  const userRawRef = useRef<HTMLStyleElement>(null);

  // Create style elements on mount
  useEffect(() => {
    adminVarsRef.current = getOrCreateStyle("prairie-admin-vars");
    adminRawRef.current = getOrCreateStyle("prairie-admin-raw-css");
    userVarsRef.current = getOrCreateStyle("prairie-user-vars");
    userRawRef.current = getOrCreateStyle("prairie-user-raw-css");

    return () => {
      // Clean up on unmount (dev HMR)
      adminVarsRef.current?.remove();
      adminRawRef.current?.remove();
      userVarsRef.current?.remove();
      userRawRef.current?.remove();
    };
  }, []);

  // Inject admin variable overrides
  useEffect(() => {
    if (adminVarsRef.current) {
      adminVarsRef.current.textContent = buildVarOverrideCSS(adminCss?.vars ?? {});
    }
  }, [adminCss?.vars]);

  // Inject admin raw CSS
  useEffect(() => {
    if (adminRawRef.current) {
      adminRawRef.current.textContent = adminCss?.rawCss ?? "";
    }
  }, [adminCss?.rawCss]);

  // Inject user variable overrides
  useEffect(() => {
    if (userVarsRef.current) {
      userVarsRef.current.textContent = buildVarOverrideCSS(vars as Record<string, string>);
    }
  }, [vars]);

  // Inject user raw CSS
  useEffect(() => {
    if (userRawRef.current) {
      userRawRef.current.textContent = customCss;
    }
  }, [customCss]);

  const adminVars = adminCss?.vars ?? {};
  const adminRawCss = adminCss?.rawCss ?? "";
  const hasOverrides =
    Object.keys(vars).length > 0 ||
    customCss.length > 0 ||
    Object.keys(adminVars).length > 0 ||
    adminRawCss.length > 0;

  return (
    <CustomThemeContext
      value={{
        vars,
        customCss,
        adminVars,
        adminRawCss,
        hasOverrides,
      }}
    >
      {children}
    </CustomThemeContext>
  );
}

export function useCustomThemeContext(): CustomThemeContextValue {
  const ctx = useContext(CustomThemeContext);
  if (!ctx) throw new Error("useCustomThemeContext must be used within CustomThemeProvider");
  return ctx;
}
