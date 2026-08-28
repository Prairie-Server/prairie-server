import { createContext, useContext, type ReactNode } from "react";

import { SettingsGroup } from "@/components/settings/SettingsGroup";
import { cn } from "@/lib/utils";
import "@/styles/admin-settings.css";

const GroupRestartContext = createContext(false);

/** True inside a `FieldGroup` where every field only applies after a restart. */
export function useGroupRestartAll(): boolean {
  return useContext(GroupRestartContext);
}

/**
 * Marks everything nested under it as restart-all without rendering any
 * group's own "Changes apply after a restart" line. Use it on a whole page
 * where every group is restart-all: say the sentence once near the page
 * title, wrap the page's groups in this provider, and pass `restartAll={false}`
 * (or omit it) on each `FieldGroup` so the per-group line does not repeat —
 * per-field restart chips stay suppressed because the groups still inherit
 * this context.
 */
export function RestartAllProvider({ children }: { children: ReactNode }) {
  return <GroupRestartContext.Provider value={true}>{children}</GroupRestartContext.Provider>;
}

export interface FieldGroupProps {
  /** Sentence-case heading, e.g. "Transcoding". */
  label: string;
  /**
   * Every field in the group only takes effect after a restart. The group says
   * so once and the fields inside drop their own chips.
   */
  restartAll?: boolean;
  /** At least one field in the group has an unsaved edit. */
  dirty?: boolean;
  /** Right-aligned controls on the heading line. */
  actions?: ReactNode;
  className?: string;
  children: ReactNode;
}

/**
 * An admin settings group: the shared `SettingsGroup` panel (the same surface
 * the user settings pages use) holding hairline-ruled `SettingFieldRow`s. The
 * admin-only concerns layer on top: the restart-all line, the unsaved-edits
 * dot, and the restart context the rows read to drop their own chips.
 */
export function FieldGroup({
  label,
  restartAll = false,
  dirty = false,
  actions,
  className,
  children,
}: FieldGroupProps) {
  // A page-level `RestartAllProvider` can already say every field in the page
  // restarts; a group nested under it inherits that without repeating its own
  // line (see `restartAll` above).
  const inheritedRestartAll = useContext(GroupRestartContext);
  const effectiveRestartAll = restartAll || inheritedRestartAll;

  const dirtyDot = dirty ? (
    <span className="inline-flex items-center" title="Unsaved changes in this group">
      <span aria-hidden="true" className="size-1.5 rounded-full bg-[var(--settings-accent)]" />
      <span className="sr-only">Unsaved changes in this group</span>
    </span>
  ) : null;

  return (
    <SettingsGroup
      title={label}
      description={restartAll ? "Changes apply after a restart" : undefined}
      actions={
        actions || dirtyDot ? (
          <>
            {actions}
            {dirtyDot}
          </>
        ) : undefined
      }
      flush
      className={cn("min-w-0", className)}
    >
      <GroupRestartContext.Provider value={effectiveRestartAll}>
        <div className="settings-field-list">{children}</div>
      </GroupRestartContext.Provider>
    </SettingsGroup>
  );
}
