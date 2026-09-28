import type { PageSectionConfig } from "@/api/types";

export interface SectionReorderEntry {
  id: string;
  position: number;
}

export function moveSectionBeforeTarget(
  sections: PageSectionConfig[],
  draggedId: string,
  targetId: string,
): PageSectionConfig[] {
  if (draggedId === targetId) {
    return sections;
  }

  const nextSections = [...sections];
  const draggedIndex = nextSections.findIndex((section) => section.id === draggedId);
  const targetIndex = nextSections.findIndex((section) => section.id === targetId);

  if (draggedIndex === -1 || targetIndex === -1) {
    return sections;
  }

  const [draggedSection] = nextSections.splice(draggedIndex, 1);
  if (!draggedSection) {
    return sections;
  }
  const insertIndex = draggedIndex < targetIndex ? targetIndex - 1 : targetIndex;
  nextSections.splice(insertIndex, 0, draggedSection);

  return nextSections;
}

export function buildSectionReorderEntries(sections: PageSectionConfig[]): SectionReorderEntry[] {
  return sections.map((section, index) => ({
    id: section.id,
    position: index,
  }));
}
