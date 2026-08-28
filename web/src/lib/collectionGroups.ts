export const UNGROUPED_LABEL = "" as const;

export function slugifyGroupSlug(title: string): string {
  return (
    title
      .toLowerCase()
      .trim()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || `group-${Date.now()}`
  );
}

export interface GroupedItemLike {
  group_id?: string | null;
}

export interface GroupDefLike {
  id: string;
  name: string;
  sort_order: number;
}

export interface GroupedSection<T> {
  id: string | null;
  name: string;
  items: T[];
}

export interface BuildSectionsOptions {
  hideEmpty?: boolean;
  ungroupedTitle?: string;
}

export function buildGroupedSections<T extends GroupedItemLike>(
  items: T[],
  groups: GroupDefLike[],
  {
    hideEmpty = false,
    ungroupedTitle = "Ungrouped",
  }: BuildSectionsOptions = {},
): GroupedSection<T>[] {
  const itemsByLabel = new Map<string, T[]>();
  for (const item of items) {
    const label = item.group_id ?? UNGROUPED_LABEL;
    const arr = itemsByLabel.get(label);
    if (arr) arr.push(item);
    else itemsByLabel.set(label, [item]);
  }

  const seenLabels = new Set<string>();
  const sections: GroupedSection<T>[] = [];

  const sortedGroups = [...groups].sort(
    (a, b) => a.sort_order - b.sort_order || a.name.localeCompare(b.name),
  );
  for (const g of sortedGroups) {
    seenLabels.add(g.id);
    const groupItems = itemsByLabel.get(g.id) ?? [];
    if (hideEmpty && groupItems.length === 0) continue;
    sections.push({ id: g.id, name: g.name, items: groupItems });
  }

  const staleLabels = Array.from(itemsByLabel.keys())
    .filter((label) => label !== UNGROUPED_LABEL && !seenLabels.has(label))
    .sort();
  for (const label of staleLabels) {
    sections.push({
      id: label,
      name: label,
      items: itemsByLabel.get(label) ?? [],
    });
  }

  const ungrouped = itemsByLabel.get(UNGROUPED_LABEL) ?? [];
  if (!hideEmpty || ungrouped.length > 0) {
    sections.push({ id: null, name: ungroupedTitle, items: ungrouped });
  }
  return sections;
}
