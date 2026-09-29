import type { FilterChipModel } from "@/lib/filterEasyMode";

interface Props {
  chip: FilterChipModel;
  onRemove: () => void;
}

function formatValue(v: FilterChipModel["value"]): string {
  if (v === null || v === undefined) return "";
  if (Array.isArray(v) && v.length === 2) {
    return `${v[0]} – ${v[1]}`;
  }
  return String(v);
}

export default function FilterChip({ chip, onRemove }: Props) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-indigo-500/30 bg-indigo-500/10 py-1 pr-1 pl-3 text-xs text-indigo-200">
      <span className="text-[10px] tracking-wider uppercase opacity-65">{chip.field}</span>
      <span className="font-semibold">{formatValue(chip.value)}</span>
      <button
        type="button"
        aria-label="remove filter"
        onClick={onRemove}
        className="px-1.5 opacity-65 hover:opacity-100"
      >
        ×
      </button>
    </span>
  );
}
