import type { HomeSectionItemsResponse, ResolvedSection, ResolvedSectionLayout } from "@/api/types";

export function collectCachedHomeSections(
  layout: ResolvedSectionLayout[],
  readCachedSection: (sectionId: string) => HomeSectionItemsResponse | undefined,
): Map<string, ResolvedSection> {
  const cachedSections = new Map<string, ResolvedSection>();

  layout.forEach((section) => {
    const cached = readCachedSection(section.id);
    if (!cached?.section) {
      return;
    }
    cachedSections.set(section.id, cached.section);
  });

  return cachedSections;
}
