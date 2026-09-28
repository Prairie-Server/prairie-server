import type { Library } from "@/api/types";

export interface LibraryReorderEntry {
  id: number;
  position: number;
}

export function buildLibraryReorderEntries(libraries: Library[]): LibraryReorderEntry[] {
  return libraries.map((lib, index) => ({ id: lib.id, position: index }));
}
