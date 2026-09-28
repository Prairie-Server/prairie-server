import type { CatalogPathRewrite } from "@/api/types";

export type PathRewriteRow = CatalogPathRewrite & { id: string };

let nextPathRewriteId = 0;

export function createEmptyPathRewrite(): PathRewriteRow {
  nextPathRewriteId += 1;
  return { id: `path-rewrite-${nextPathRewriteId}`, from: "", to: "" };
}

export function updatePathRewrite(
  rewrites: PathRewriteRow[],
  index: number,
  field: keyof CatalogPathRewrite,
  value: string,
): PathRewriteRow[] {
  return rewrites.map((rewrite, i) => (i === index ? { ...rewrite, [field]: value } : rewrite));
}

export function addEmptyPathRewrite(rewrites: PathRewriteRow[]): PathRewriteRow[] {
  return [...rewrites, createEmptyPathRewrite()];
}

export function removePathRewrite(rewrites: PathRewriteRow[], index: number): PathRewriteRow[] {
  return rewrites.length === 1
    ? [createEmptyPathRewrite()]
    : rewrites.filter((_, i) => i !== index);
}
