import { useId } from "react";

export function FieldGroup({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  const labelId = useId();
  return (
    <div
      role="group"
      aria-labelledby={labelId}
      className="surface-panel rounded-2xl border-0 p-4 sm:p-5"
    >
      <div
        id={labelId}
        className="text-muted-foreground mb-3 text-xs font-semibold tracking-[0.22em] uppercase"
      >
        {label}
      </div>
      <div className="divide-border divide-y">{children}</div>
    </div>
  );
}
