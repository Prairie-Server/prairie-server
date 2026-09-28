import { useState } from "react";
import type { FilterChipModel } from "@/lib/filterEasyMode";

const FIELDS = [
  { value: "", label: "Field…" },
  { value: "genre", label: "Genre" },
  { value: "year", label: "Year" },
  { value: "rating_imdb", label: "Rating (IMDb)" },
  { value: "director", label: "Director" },
  { value: "studio", label: "Studio" },
  { value: "cast", label: "Cast" },
  { value: "library", label: "Library" },
  { value: "watched", label: "Has been watched" },
  { value: "language", label: "Language" },
  { value: "runtime", label: "Runtime (min)" },
  { value: "keyword", label: "Keyword" },
];

const OPS = [
  { value: "is", label: "is" },
  { value: "is_not", label: "is not" },
  { value: "gte", label: "≥" },
  { value: "lte", label: "≤" },
  { value: "between", label: "between" },
  { value: "contains", label: "contains" },
];

interface Props {
  open: boolean;
  onAdd: (chip: FilterChipModel) => void;
  onCancel: () => void;
}

export default function AddFilterPopover({ open, onAdd, onCancel }: Props) {
  const [field, setField] = useState("");
  const [op, setOp] = useState("contains");
  const [value, setValue] = useState("");
  if (!open) return null;
  return (
    <div className="rounded-lg border border-indigo-500/40 bg-zinc-900 p-3 shadow-lg" role="dialog">
      <div className="grid grid-cols-3 gap-2">
        <label className="flex flex-col text-[11px] text-white/70">
          Field
          <select
            value={field}
            onChange={(e) => setField(e.target.value)}
            className="rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white"
          >
            {FIELDS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col text-[11px] text-white/70">
          Operator
          <select
            value={op}
            onChange={(e) => setOp(e.target.value)}
            className="rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white"
          >
            {OPS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col text-[11px] text-white/70">
          Value
          <input
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder="value"
            className="rounded border border-white/15 bg-white/5 px-2 py-1 text-xs text-white"
          />
        </label>
      </div>
      <div className="mt-3 flex justify-end gap-2">
        <button
          type="button"
          onClick={onCancel}
          className="rounded border border-white/15 px-2 py-1 text-xs text-white/70"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() => {
            if (!field) return;
            onAdd({ field, op, value });
            setField("");
            setOp("contains");
            setValue("");
          }}
          className="rounded bg-indigo-600 px-2 py-1 text-xs text-white"
        >
          Add
        </button>
      </div>
    </div>
  );
}
