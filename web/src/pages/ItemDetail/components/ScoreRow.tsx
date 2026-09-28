import { Star } from "lucide-react";

interface ScoreRowProps {
  ratingImdb?: number | null;
  ratingRtCritic?: number | null;
  ratingRtAudience?: number | null;
}

export default function ScoreRow({ ratingImdb, ratingRtCritic, ratingRtAudience }: ScoreRowProps) {
  if (ratingImdb == null && ratingRtCritic == null && ratingRtAudience == null) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-5">
      {ratingImdb != null && (
        <div className="flex items-center gap-1.5">
          <Star className="text-primary size-4 fill-current" />
          <span className="text-primary text-[15px] font-bold">{ratingImdb.toFixed(1)}</span>
          <span className="text-muted-foreground/50 text-xs">/10</span>
        </div>
      )}
      {ratingRtCritic != null && (
        <div className="flex items-center gap-1.5">
          <span className="text-sm">🍅</span>
          <span className="text-muted-foreground text-[13px] font-medium">{ratingRtCritic}%</span>
        </div>
      )}
      {ratingRtAudience != null && (
        <div className="flex items-center gap-1.5">
          <span className="text-sm">🍿</span>
          <span className="text-muted-foreground text-[13px] font-medium">{ratingRtAudience}%</span>
        </div>
      )}
    </div>
  );
}
