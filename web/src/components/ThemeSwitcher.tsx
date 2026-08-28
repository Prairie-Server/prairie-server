import { useTheme } from "@/hooks/useTheme";
import { CURATED_THEME_IDS, THEMES } from "@/lib/themes";
import { DropdownMenuItem } from "@/components/ui/dropdown-menu";
import { Check } from "lucide-react";

export default function ThemeSwitcher() {
  const { theme, setTheme, previewTheme, resetPreviewTheme } = useTheme();

  return (
    <div className="grid gap-1.5">
      {CURATED_THEME_IDS.map((id) => {
        const def = THEMES[id];
        const isActive = theme === id;
        return (
          <DropdownMenuItem
            key={id}
            onClick={() => setTheme(id)}
            onFocus={() => previewTheme(id)}
            onBlur={resetPreviewTheme}
            onMouseEnter={() => previewTheme(id)}
            onMouseLeave={resetPreviewTheme}
            className={`rounded-xl border px-2.5 py-2.5 ${isActive ? "border-primary/35 bg-accent/70" : "border-transparent bg-transparent"} flex items-start gap-3`}
          >
            <div
              className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border"
              style={{
                backgroundColor: def.previewBg,
                borderColor: isActive
                  ? def.previewAccent
                  : "color-mix(in srgb, var(--border) 70%, transparent)",
              }}
            >
              <div className="flex items-center gap-1.5">
                <div
                  className="h-3 w-3 rounded-full"
                  style={{ backgroundColor: def.previewAccent }}
                />
                <div className="h-3 w-1.5 rounded-full bg-white/60" />
              </div>
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate text-sm font-medium">
                  {def.label}
                </span>
                {isActive && (
                  <Check className="text-primary h-4 w-4 shrink-0" />
                )}
              </div>
              {def.description ? (
                <p className="text-muted-foreground mt-0.5 line-clamp-2 text-xs leading-relaxed">
                  {def.description}
                </p>
              ) : null}
            </div>
          </DropdownMenuItem>
        );
      })}
    </div>
  );
}
