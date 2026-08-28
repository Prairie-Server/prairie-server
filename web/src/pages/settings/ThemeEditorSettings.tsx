import { useCallback } from "react";
import { Paintbrush, Code, Store, RotateCcw } from "lucide-react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "@/components/ui/tabs";
import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { TokenEditor } from "@/components/theme/TokenEditor";
import { RawCssEditor } from "@/components/theme/RawCssEditor";
import { CatalogBrowser } from "@/components/theme/CatalogBrowser";
import { ImportExportBar } from "@/components/theme/ImportExportBar";
import { ThemePreviewCard } from "@/components/theme/ThemePreviewCard";
import { useCustomTheme } from "@/hooks/useCustomTheme";
import type { ThemeVarOverrides } from "@/hooks/useCustomTheme";
import { useTheme } from "@/hooks/useTheme";
import type { ThemeId } from "@/lib/themes";

export default function ThemeEditorSettings() {
  const { theme, setTheme } = useTheme();
  const {
    vars,
    customCss,
    setVar,
    resetVar,
    setCustomCss,
    resetAll,
    importOverrides,
  } = useCustomTheme();

  const hasOverrides = Object.keys(vars).length > 0 || customCss.length > 0;

  const handleCatalogInstall = useCallback(
    (entry: {
      baseTheme: string;
      vars: Record<string, string>;
      customCss: string;
    }) => {
      setTheme(entry.baseTheme as ThemeId);
      importOverrides(entry.vars, entry.customCss);
    },
    [setTheme, importOverrides],
  );

  const handleImport = useCallback(
    (newVars: ThemeVarOverrides, css: string, baseTheme: ThemeId) => {
      setTheme(baseTheme);
      importOverrides(newVars, css);
    },
    [setTheme, importOverrides],
  );

  return (
    <div className="space-y-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold tracking-tight sm:text-3xl">
            Theme Editor
          </h2>
          <p className="text-muted-foreground mt-1 text-sm">
            Customize colors, fonts, and styles. Changes are layered on top of
            your active theme.
          </p>
        </div>
        <ImportExportBar
          currentTheme={theme}
          vars={vars}
          customCss={customCss}
          onImport={handleImport}
        />
      </div>

      {/* Live preview */}
      <SettingsGroup
        title="Preview"
        description="Live preview of your customizations."
      >
        <ThemePreviewCard vars={vars} />
        {hasOverrides && (
          <div className="flex justify-end">
            <button
              type="button"
              onClick={resetAll}
              className="text-muted-foreground hover:text-destructive inline-flex items-center gap-1.5 text-xs font-medium transition-colors"
            >
              <RotateCcw className="h-3 w-3" />
              Reset all customizations
            </button>
          </div>
        )}
      </SettingsGroup>

      {/* Tabbed editor */}
      <Tabs defaultValue="tokens">
        <TabsList>
          <TabsTrigger value="tokens" className="gap-1.5">
            <Paintbrush className="h-3.5 w-3.5" />
            Tokens
          </TabsTrigger>
          <TabsTrigger value="css" className="gap-1.5">
            <Code className="h-3.5 w-3.5" />
            Custom CSS
          </TabsTrigger>
          <TabsTrigger value="catalog" className="gap-1.5">
            <Store className="h-3.5 w-3.5" />
            Catalog
          </TabsTrigger>
        </TabsList>

        <TabsContent value="tokens">
          <SettingsGroup
            title="Token Overrides"
            description="Override individual design tokens. Changes layer on top of your active theme and apply instantly."
          >
            <TokenEditor vars={vars} onSetVar={setVar} onResetVar={resetVar} />
          </SettingsGroup>
        </TabsContent>

        <TabsContent value="css">
          <SettingsGroup
            title="Custom CSS"
            description="Write arbitrary CSS for advanced customization."
          >
            <RawCssEditor value={customCss} onChange={setCustomCss} />
          </SettingsGroup>
        </TabsContent>

        <TabsContent value="catalog">
          <SettingsGroup
            title="Community Themes"
            description="Browse and install themes created by the community."
          >
            <CatalogBrowser onInstall={handleCatalogInstall} />
          </SettingsGroup>
        </TabsContent>
      </Tabs>
    </div>
  );
}
