import { useEffect, useState } from "react";
import { previewSection, type PreviewResponse } from "@/lib/recipes";

interface Props {
  sectionType: string;
  config: Record<string, unknown>;
  itemLimit: number;
  libraryID?: number;
}

export default function PreviewBox({ sectionType, config, itemLimit, libraryID }: Props) {
  const [data, setData] = useState<PreviewResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    const handle = setTimeout(async () => {
      setLoading(true);
      setError(null);
      try {
        const res = await previewSection({
          section_type: sectionType,
          config,
          item_limit: itemLimit,
          library_id: libraryID,
        });
        setData(res);
      } catch (e) {
        setError(String(e));
      } finally {
        setLoading(false);
      }
    }, 300);
    return () => clearTimeout(handle);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sectionType, JSON.stringify(config), itemLimit, libraryID]);

  return (
    <div className="rounded border-l-2 border-indigo-500 bg-indigo-500/10 px-3 py-2 text-xs">
      <div className="mb-1 text-[10px] tracking-wider uppercase opacity-65">
        {loading ? "Loading…" : error ? "Error" : `Preview · ${data?.total_count ?? 0} items match`}
      </div>
      {error && <div className="text-red-400">{error}</div>}
      {!error && data && (
        <div className="truncate font-mono text-[11px] opacity-90">
          {data.items
            .slice(0, 10)
            .map((i) => i.title ?? i.content_id)
            .join(" · ") || "no matches"}
        </div>
      )}
    </div>
  );
}
