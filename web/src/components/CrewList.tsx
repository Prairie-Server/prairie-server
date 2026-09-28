import { ChevronLeft, ChevronRight } from "lucide-react";
import { PersonCard } from "@/components/CastCarousel";
import type { CrewMember } from "@/api/types";
import { useCarouselEmbla } from "@/hooks/useCarouselEmbla";
import { buildPersonCatalogHref } from "@/pages/catalogSearchParams";
import { cn } from "@/lib/utils";

interface CrewListProps {
  crew: CrewMember[];
  limit?: number;
  fullBleed?: boolean;
}

/** Jobs to display, in order. Others are ignored unless none match. */
const DISPLAY_JOBS = ["Director", "Writer", "Producer", "Creator", "Executive Producer"] as const;

function normalizeJob(job: string): string {
  return job.trim().toLowerCase();
}

function pickCrew(crew: CrewMember[], limit: number): CrewMember[] {
  const preferred = new Map<string, number>();
  DISPLAY_JOBS.forEach((job, index) => preferred.set(normalizeJob(job), index));

  const ranked = crew
    .map((member, index) => ({
      member,
      index,
      rank: preferred.get(normalizeJob(member.job)),
    }))
    .filter((entry) => entry.rank != null)
    .sort((a, b) => a.rank! - b.rank! || a.index - b.index)
    .map((entry) => entry.member);

  const source = ranked.length > 0 ? ranked : crew;
  const seen = new Set<string>();
  const out: CrewMember[] = [];
  for (const member of source) {
    const key = `${member.person_id || member.name}|${member.job}`;
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(member);
    if (out.length >= limit) break;
  }
  return out;
}

/**
 * Portrait crew rail matching CastCarousel card metrics (avoids tiny cast vs
 * oversized definition-list crew imbalance).
 */
export default function CrewList({ crew, limit = 16, fullBleed = false }: CrewListProps) {
  const { emblaRef, canScrollPrev, canScrollNext, scrollPrev, scrollNext } = useCarouselEmbla();

  if (crew.length === 0) return null;

  const visible = pickCrew(crew, limit);
  if (visible.length === 0) return null;

  return (
    <div>
      <h2 className="mb-5 text-xl font-semibold tracking-tight">Crew</h2>
      <div className="group/carousel relative">
        {canScrollPrev && (
          <button
            type="button"
            onClick={scrollPrev}
            className={cn(
              "from-background/90 absolute top-0 bottom-0 z-10 flex h-11 w-11 items-center justify-center self-center bg-gradient-to-r to-transparent opacity-0 transition-opacity duration-200 group-hover/carousel:opacity-100 focus-visible:opacity-100",
              fullBleed ? "left-4 sm:left-6 lg:left-10 xl:left-12" : "left-0",
            )}
            aria-label="Scroll left"
          >
            <ChevronLeft className="text-foreground h-6 w-6" />
          </button>
        )}

        <div
          ref={emblaRef}
          className={cn(
            "embla__viewport overflow-hidden",
            fullBleed && "pr-4 sm:pr-6 lg:pr-10 xl:pr-12",
          )}
        >
          <ul
            role="list"
            className={cn(
              "embla__container flex cursor-grab list-none gap-3",
              fullBleed && "pl-4 sm:pl-6 lg:pl-10 xl:pl-12",
            )}
          >
            {visible.map((member) => {
              const href = member.person_id ? buildPersonCatalogHref(member.person_id) : null;
              return (
                <li
                  key={`${member.person_id || member.name}-${member.job}`}
                  className="embla__slide shrink-0"
                >
                  <PersonCard
                    name={member.name}
                    subtitle={member.job}
                    photoUrl={member.photo_url}
                    href={href}
                  />
                </li>
              );
            })}
          </ul>
        </div>

        {canScrollNext && (
          <button
            type="button"
            onClick={scrollNext}
            className={cn(
              "from-background/90 absolute top-0 bottom-0 z-10 flex h-11 w-11 items-center justify-center self-center bg-gradient-to-l to-transparent opacity-0 transition-opacity duration-200 group-hover/carousel:opacity-100 focus-visible:opacity-100",
              fullBleed ? "right-4 sm:right-6 lg:right-10 xl:right-12" : "right-0",
            )}
            aria-label="Scroll right"
          >
            <ChevronRight className="text-foreground h-6 w-6" />
          </button>
        )}
      </div>
    </div>
  );
}
