import { ChevronLeft, ChevronRight } from "lucide-react";
import { PersonCard } from "@/components/CastCarousel";
import type { CrewMember } from "@/api/types";
import { useCarouselEmbla } from "@/hooks/useCarouselEmbla";
import { buildPersonCatalogHref } from "@/pages/catalogSearchParams";
import { cn } from "@/lib/utils";
import { useT } from "@/i18n";

interface CrewListProps {
  crew: CrewMember[];
  limit?: number;
  fullBleed?: boolean;
  /** Jobs to rank first, in order; all crew is shown when none match. Defaults to DISPLAY_JOBS. */
  jobs?: readonly string[];
}

/**
 * Jobs to display, in order, when the page doesn't choose its own. Creator is
 * left out on purpose: a series page names its own jobs (see SeriesContent), and
 * season and episode pages keep the series' creators off their crew rail.
 */
const DISPLAY_JOBS = ["Director", "Writer", "Producer", "Executive Producer"] as const;

function normalizeJob(job: string): string {
  return job.trim().toLowerCase();
}

function pickCrew(crew: CrewMember[], limit: number, jobs: readonly string[]): CrewMember[] {
  const preferred = new Map<string, number>();
  jobs.forEach((job, index) => preferred.set(normalizeJob(job), index));

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
export default function CrewList({
  crew,
  limit = 16,
  fullBleed = false,
  jobs = DISPLAY_JOBS,
}: CrewListProps) {
  const t = useT();
  const { emblaRef, canScrollPrev, canScrollNext, scrollPrev, scrollNext } = useCarouselEmbla();

  if (crew.length === 0) return null;

  const visible = pickCrew(crew, limit, jobs);
  if (visible.length === 0) return null;

  return (
    <div>
      <h2 className="mb-5 text-xl font-semibold tracking-tight">{t("web.detail.castCrew.crew")}</h2>
      <div className="group/carousel relative">
        {canScrollPrev && (
          <button
            type="button"
            onClick={scrollPrev}
            className={cn(
              "from-background/90 absolute top-0 bottom-0 z-10 flex h-11 w-11 items-center justify-center self-center bg-gradient-to-r to-transparent opacity-0 transition-opacity duration-200 group-hover/carousel:opacity-100 focus-visible:opacity-100 rtl:bg-gradient-to-l",
              fullBleed ? "start-4 sm:start-6 lg:start-10 xl:start-12" : "start-0",
            )}
            aria-label={t("web.detail.castCrew.scrollLeft")}
          >
            <ChevronLeft className="text-foreground h-6 w-6" />
          </button>
        )}

        <div
          ref={emblaRef}
          className={cn(
            "embla__viewport overflow-hidden",
            fullBleed && "pe-4 sm:pe-6 lg:pe-10 xl:pe-12",
          )}
        >
          <ul
            role="list"
            className={cn(
              "embla__container flex cursor-grab list-none gap-3",
              fullBleed && "ps-4 sm:ps-6 lg:ps-10 xl:ps-12",
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
              "from-background/90 absolute top-0 bottom-0 z-10 flex h-11 w-11 items-center justify-center self-center bg-gradient-to-l to-transparent opacity-0 transition-opacity duration-200 group-hover/carousel:opacity-100 focus-visible:opacity-100 rtl:bg-gradient-to-r",
              fullBleed ? "end-4 sm:end-6 lg:end-10 xl:end-12" : "end-0",
            )}
            aria-label={t("web.detail.castCrew.scrollRight")}
          >
            <ChevronRight className="text-foreground h-6 w-6" />
          </button>
        )}
      </div>
    </div>
  );
}
