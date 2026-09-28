import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { getDefaultQuerySortOrder, getQuerySortOptions } from "@/lib/querySortOptions";

interface Filters {
  type: string;
  sort: string;
  order: string;
}

interface FilterBarProps {
  filters: Filters;
  onChange: (filters: Filters) => void;
}

export default function FilterBar({ filters, onChange }: FilterBarProps) {
  const sortOptions = getQuerySortOptions(false);

  return (
    <div className="flex flex-wrap gap-3">
      <Select value={filters.type} onValueChange={(v) => onChange({ ...filters, type: v })}>
        <SelectTrigger className="w-32">
          <SelectValue placeholder="Type" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All</SelectItem>
          <SelectItem value="movie">Movies</SelectItem>
          <SelectItem value="series">Series</SelectItem>
        </SelectContent>
      </Select>

      <Select
        value={filters.sort}
        onValueChange={(v) => onChange({ ...filters, sort: v, order: getDefaultQuerySortOrder(v) })}
      >
        <SelectTrigger className="w-40">
          <SelectValue placeholder="Sort by" />
        </SelectTrigger>
        <SelectContent>
          {sortOptions.map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select value={filters.order} onValueChange={(v) => onChange({ ...filters, order: v })}>
        <SelectTrigger className="w-28">
          <SelectValue placeholder="Order" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="desc">Desc</SelectItem>
          <SelectItem value="asc">Asc</SelectItem>
        </SelectContent>
      </Select>
    </div>
  );
}
