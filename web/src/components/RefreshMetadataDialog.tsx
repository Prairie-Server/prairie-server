import { Loader2, RefreshCw, RotateCcw, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { RefreshItemMetadataMode } from "@/hooks/queries/items";

interface RefreshMetadataDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (mode: RefreshItemMetadataMode) => void;
  isPending?: boolean;
}

export default function RefreshMetadataDialog({
  open,
  onOpenChange,
  onConfirm,
  isPending = false,
}: RefreshMetadataDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Refresh Metadata</DialogTitle>
          <DialogDescription>
            Choose whether to refresh the existing item or rebuild it from the
            files on disk.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <button
            type="button"
            disabled={isPending}
            onClick={() => onConfirm("quick")}
            className="border-border bg-surface hover:bg-surface/80 flex w-full items-start gap-3 rounded-xl border p-4 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60"
          >
            {isPending ? (
              <Loader2 className="text-muted-foreground mt-0.5 size-5 animate-spin" />
            ) : (
              <RefreshCw className="text-muted-foreground mt-0.5 size-5" />
            )}
            <div className="space-y-1">
              <div className="text-sm font-semibold">Quick Refresh</div>
              <div className="text-muted-foreground text-sm">
                Keep the current item and refresh metadata using the existing
                scan scope.
              </div>
            </div>
          </button>

          <button
            type="button"
            disabled={isPending}
            onClick={() => onConfirm("complete")}
            className="border-border bg-surface hover:bg-surface/80 flex w-full items-start gap-3 rounded-xl border p-4 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60"
          >
            {isPending ? (
              <Loader2 className="text-muted-foreground mt-0.5 size-5 animate-spin" />
            ) : (
              <RotateCcw className="text-muted-foreground mt-0.5 size-5" />
            )}
            <div className="space-y-1">
              <div className="text-sm font-semibold">Complete Refresh</div>
              <div className="text-muted-foreground text-sm">
                Clear the current match, re-scan, and rebuild the item from disk
                context. This can recreate the item with a new ID or type.
              </div>
            </div>
          </button>
        </div>

        <div className="flex justify-end">
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={isPending}
          >
            <X />
            Cancel
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
