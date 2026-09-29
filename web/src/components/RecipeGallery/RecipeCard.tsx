import type { GalleryPreset, Category } from "@/lib/recipes";

interface Props {
  preset: GalleryPreset;
  category: Category;
  onPick: () => void;
}

export default function RecipeCard({ preset, category, onPick }: Props) {
  return (
    <button
      type="button"
      onClick={onPick}
      className="rounded-lg border border-white/10 bg-white/5 p-3 text-left transition-colors hover:border-indigo-500"
      aria-label={preset.display_name}
    >
      <div className="text-lg">{preset.icon}</div>
      <div className="mt-1 text-sm font-semibold">{preset.display_name}</div>
      <div className="mt-1 text-xs leading-tight text-white/60">{preset.description_short}</div>
      <div className="mt-2 text-[10px] tracking-wider text-white/45 uppercase">
        {category.replace("_", " ")}
      </div>
    </button>
  );
}
