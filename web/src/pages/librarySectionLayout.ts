import type { ResolvedSection } from "@/api/types";

export interface LibrarySectionLayout {
  hero: ResolvedSection | null;
  rows: ResolvedSection[];
}

export function splitLibrarySections(sections: ResolvedSection[]): LibrarySectionLayout {
  const hero = sections.find((section) => section.featured && section.items.length > 0) ?? null;
  const rows = sections.filter((section) => section.items.length > 0 && section.id !== hero?.id);

  return { hero, rows };
}
