import type { ReactNode } from "react";

import { cn } from "@/lib/utils";

interface SettingsGroupProps {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}

export function SettingsGroup({
  title,
  description,
  children,
  className,
}: SettingsGroupProps) {
  return (
    <section
      className={cn(
        "surface-panel rounded-[1.7rem] border-0 px-4 py-5 shadow-none sm:px-6",
        className,
      )}
    >
      <div className="space-y-1">
        <h3 className="text-foreground text-sm font-semibold tracking-tight">
          {title}
        </h3>
        {description ? (
          <p className="text-muted-foreground text-[13px] leading-relaxed">
            {description}
          </p>
        ) : null}
      </div>
      <div className="mt-5 space-y-4">{children}</div>
    </section>
  );
}
