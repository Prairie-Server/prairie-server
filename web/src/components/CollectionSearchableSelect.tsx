import * as React from "react";
import { Popover as PopoverPrimitive } from "radix-ui";
import { Check, ChevronsUpDown } from "lucide-react";

import { cn } from "@/lib/utils";
import type { CollectionOption } from "@/hooks/queries/useAllUserCollections";

interface CollectionSearchableSelectProps {
  /** The full list of available collection options. */
  options: CollectionOption[];
  /** Currently selected collection ID (empty string = nothing selected). */
  value: string;
  /** Called when the user picks a collection or clears the selection. */
  onChange: (value: string) => void;
  placeholder?: string;
  disabled?: boolean;
  /** Show a loading skeleton while options are being fetched. */
  isLoading?: boolean;
}

export function CollectionSearchableSelect({
  options,
  value,
  onChange,
  placeholder = "Choose collection",
  disabled = false,
  isLoading = false,
}: CollectionSearchableSelectProps) {
  const [open, setOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");

  const filtered = React.useMemo(() => {
    if (!search) return options;
    const lower = search.toLowerCase();
    return options.filter(
      (opt) => opt.title.toLowerCase().includes(lower) || opt.group.toLowerCase().includes(lower),
    );
  }, [options, search]);

  // Group filtered options by their `group` field, preserving insertion order.
  const grouped = React.useMemo(() => {
    const map = new Map<string, CollectionOption[]>();
    for (const opt of filtered) {
      const existing = map.get(opt.group);
      if (existing) {
        existing.push(opt);
      } else {
        map.set(opt.group, [opt]);
      }
    }
    return map;
  }, [filtered]);

  const selectedOption = React.useMemo(() => options.find((o) => o.id === value), [options, value]);

  const displayText = selectedOption
    ? `${selectedOption.title} (${selectedOption.group})`
    : placeholder;

  return (
    <PopoverPrimitive.Root open={open} onOpenChange={setOpen} modal={false}>
      <PopoverPrimitive.Trigger asChild disabled={disabled}>
        <button
          type="button"
          role="combobox"
          aria-expanded={open}
          className={cn(
            "border-input bg-background ring-offset-background placeholder:text-muted-foreground focus:ring-ring flex h-9 w-full items-center justify-between rounded-md border px-3 py-2 text-sm shadow-xs focus:ring-1 focus:outline-none disabled:cursor-not-allowed disabled:opacity-50",
            !value && "text-muted-foreground",
          )}
        >
          <span className="truncate">{isLoading ? "Loading..." : displayText}</span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </button>
      </PopoverPrimitive.Trigger>

      <PopoverPrimitive.Portal>
        <PopoverPrimitive.Content
          align="start"
          sideOffset={4}
          className="bg-popover text-popover-foreground z-50 w-[var(--radix-popover-trigger-width)] rounded-md border shadow-md"
          onOpenAutoFocus={(e) => e.preventDefault()}
        >
          <div className="p-2">
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Search collections..."
              className="border-input bg-background placeholder:text-muted-foreground flex h-8 w-full rounded-md border px-2 text-sm outline-none"
              autoFocus
            />
          </div>

          <div className="max-h-60 overflow-y-auto p-1">
            {isLoading ? (
              <p className="text-muted-foreground py-4 text-center text-sm">Loading...</p>
            ) : filtered.length === 0 ? (
              <p className="text-muted-foreground py-4 text-center text-sm">No collections found</p>
            ) : (
              <>
                {/* Clear / placeholder option */}
                <button
                  type="button"
                  className={cn(
                    "hover:bg-accent hover:text-accent-foreground relative flex w-full cursor-pointer items-center rounded-sm px-2 py-1.5 text-sm select-none",
                    !value && "font-medium",
                  )}
                  onClick={() => {
                    onChange("");
                    setOpen(false);
                    setSearch("");
                  }}
                >
                  <Check
                    className={cn("mr-2 h-4 w-4 shrink-0", value ? "opacity-0" : "opacity-100")}
                  />
                  <span className="text-muted-foreground italic">Choose collection</span>
                </button>

                {Array.from(grouped.entries()).map(([groupName, items]) => (
                  <div key={groupName}>
                    {/* Group header */}
                    <div className="text-muted-foreground px-2 pt-2 pb-1 text-xs font-semibold">
                      {groupName}
                    </div>
                    {items.map((opt) => (
                      <button
                        type="button"
                        key={opt.id}
                        className={cn(
                          "hover:bg-accent hover:text-accent-foreground relative flex w-full cursor-pointer items-center rounded-sm px-2 py-1.5 text-sm select-none",
                          value === opt.id && "font-medium",
                        )}
                        onClick={() => {
                          onChange(opt.id);
                          setOpen(false);
                          setSearch("");
                        }}
                      >
                        <Check
                          className={cn(
                            "mr-2 h-4 w-4 shrink-0",
                            value === opt.id ? "opacity-100" : "opacity-0",
                          )}
                        />
                        {opt.title}
                      </button>
                    ))}
                  </div>
                ))}
              </>
            )}
          </div>
        </PopoverPrimitive.Content>
      </PopoverPrimitive.Portal>
    </PopoverPrimitive.Root>
  );
}
