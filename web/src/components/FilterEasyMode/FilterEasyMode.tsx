import { useState } from "react";
import FilterChip from "./FilterChip";
import AddFilterPopover from "./AddFilterPopover";
import QuickStartTemplates from "./QuickStartTemplates";
import {
  chipsToFilterConfig,
  filterConfigToChips,
  type FilterChipModel,
} from "@/lib/filterEasyMode";
import type { FilterConfig } from "@/api/types";

interface Props {
  initialConfig: FilterConfig;
  onChange: (next: FilterConfig) => void;
}

export default function FilterEasyMode({ initialConfig, onChange }: Props) {
  const conv = filterConfigToChips(initialConfig);
  const [chips, setChips] = useState<FilterChipModel[]>(
    conv.kind === "compatible" ? conv.chips : [],
  );
  const [matchMode, setMatchMode] = useState<"all" | "any">(
    conv.kind === "compatible" ? conv.matchMode : "all",
  );
  const [popoverOpen, setPopoverOpen] = useState(false);

  function emit(nextChips: FilterChipModel[], nextMode: "all" | "any") {
    onChange(chipsToFilterConfig(nextChips, nextMode));
  }

  function setAndEmit(nextChips: FilterChipModel[], nextMode: "all" | "any" = matchMode) {
    setChips(nextChips);
    setMatchMode(nextMode);
    emit(nextChips, nextMode);
  }

  return (
    <div>
      <div className="mb-2 text-[11px] tracking-wider uppercase opacity-60">
        Quick-start templates
      </div>
      <QuickStartTemplates onPick={(picked) => setAndEmit(picked, matchMode)} />

      <div className="mt-5 mb-2 text-xs opacity-80">
        Match
        <select
          className="mr-2 ml-2 rounded border border-white/15 bg-white/5 px-2 py-0.5 text-xs"
          value={matchMode}
          onChange={(e) => {
            const m = e.target.value as "all" | "any";
            setAndEmit(chips, m);
          }}
        >
          <option value="all">all</option>
          <option value="any">any</option>
        </select>
        of these conditions
      </div>

      <div className="flex min-h-[60px] flex-wrap items-center gap-2 rounded-lg border border-dashed border-white/10 bg-white/5 p-3">
        {chips.map((c, i) => (
          <FilterChip
            key={`${c.field}-${i}`}
            chip={c}
            onRemove={() => {
              const next = chips.filter((_, j) => j !== i);
              setAndEmit(next, matchMode);
            }}
          />
        ))}
        <button
          type="button"
          onClick={() => setPopoverOpen(true)}
          className="rounded-full border border-dashed border-white/20 bg-white/5 px-3 py-1 text-xs"
        >
          + Add filter
        </button>
      </div>

      <div className="mt-2">
        <AddFilterPopover
          open={popoverOpen}
          onCancel={() => setPopoverOpen(false)}
          onAdd={(chip) => {
            const next = [...chips, chip];
            setAndEmit(next, matchMode);
            setPopoverOpen(false);
          }}
        />
      </div>
    </div>
  );
}
