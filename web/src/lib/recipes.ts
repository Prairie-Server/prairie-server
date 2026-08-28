import { api } from "../api/client";

export type Category =
  | "library_staples"
  | "personalized"
  | "discovery"
  | "editorial"
  | "seasonal"
  | "mood"
  | "hand_picked"
  | "social"
  | "custom";

export interface GalleryPreset {
  key: string;
  display_name: string;
  icon: string;
  description_short: string;
  description_long?: string;
  default_params: Record<string, unknown>;
}

export interface RecipeDefinition {
  type: string;
  category: Category;
  presets: GalleryPreset[];
  avoid_duplicates: boolean;
  supports_rotation: boolean;
  admin_only: boolean;
}

export interface RecipeCatalogResponse {
  categories: Partial<Record<Category, RecipeDefinition[]>>;
}

export interface Candidate {
  value: string;
  display_name: string;
  subtitle?: string;
}

export interface PreviewRequest {
  section_type: string;
  config: Record<string, unknown>;
  item_limit?: number;
  library_id?: number;
  library_ids?: number[];
}

export interface PreviewResponse {
  items: Array<{ content_id: string; title?: string; poster_path?: string }>;
  total_count: number;
}

export async function fetchRecipeCatalog(): Promise<RecipeCatalogResponse> {
  return api<RecipeCatalogResponse>("/sections/recipes");
}

export async function fetchCandidates(
  recipeType: string,
): Promise<Candidate[]> {
  const body = await api<{ candidates: Candidate[] }>(
    `/sections/recipes/${encodeURIComponent(recipeType)}/candidates`,
  );
  return body.candidates;
}

export async function previewSection(
  req: PreviewRequest,
): Promise<PreviewResponse> {
  return api<PreviewResponse>("/admin/sections/preview", {
    method: "POST",
    body: JSON.stringify(req),
  });
}
