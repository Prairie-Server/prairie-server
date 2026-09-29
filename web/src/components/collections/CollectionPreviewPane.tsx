import type { CollectionPreviewResponse } from "@/api/types";

interface CollectionPreviewPaneProps {
  preview?: CollectionPreviewResponse;
  enabled: boolean;
  isLoading?: boolean;
}

export default function CollectionPreviewPane({
  preview,
  enabled,
  isLoading = false,
}: CollectionPreviewPaneProps) {
  if (!enabled) {
    return (
      <div className="text-muted-foreground rounded-lg border border-dashed px-4 py-5 text-sm">
        Switch this collection to smart mode to preview matching titles live.
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="text-muted-foreground rounded-lg border px-4 py-5 text-sm">
        Loading preview…
      </div>
    );
  }

  const items = preview?.items ?? [];
  const total = preview?.total ?? 0;

  return (
    <div className="space-y-3 rounded-lg border px-4 py-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium">Preview</p>
          <p className="text-muted-foreground text-xs">
            Matching items update as the rules change.
          </p>
        </div>
        <div className="text-sm font-medium">{total}</div>
      </div>

      {items.length === 0 ? (
        <div className="text-muted-foreground text-sm">No titles match the current query.</div>
      ) : (
        <div className="space-y-2">
          {items.map((item) => (
            <div key={item.content_id} className="flex items-center justify-between gap-3 text-sm">
              <span className="truncate">{item.title}</span>
              <span className="text-muted-foreground uppercase">{item.type}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
