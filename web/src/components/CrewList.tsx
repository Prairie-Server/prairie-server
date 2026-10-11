import { ChevronLeft, ChevronRight } from "lucide-react";
import { PersonCard } from "@/components/CastCarousel";
import type { CrewMember } from "@/api/types";
import { useCarouselEmbla } from "@/hooks/useCarouselEmbla";
import { buildPersonCatalogHref } from "@/pages/catalogSearchParams";
import { cn } from "@/lib/utils";

interface CrewListProps {
  crew: CrewMember[];
  /** Jobs to list, in order. Others are ignored. Defaults to Director, Writer and Producer. */
  jobs?: readonly string[];
}

/** Jobs to display, in order. Others are ignored unless none match. */
const DISPLAY_JOBS = ["Director", "Writer", "Producer", "Creator", "Executive Producer"] as const;

function normalizeJob(job: string): string {
  return job.trim().toLowerCase();
}

/** Jobs to display, in order, when the page doesn't choose its own. */
const DISPLAY_JOBS = ["Director", "Writer", "Producer"] as const;

/**
 * Lists a title's crew by job, one row per job in `jobs` order, each person
 * once and linked to their person page. Renders nothing when no listed job has
 * credits.
 */
export default function CrewList({ crew, jobs = DISPLAY_JOBS }: CrewListProps) {
  if (crew.length === 0) return null;

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

  const entries = jobs
    .filter((job) => grouped.has(job))
    .map((job) => ({
      job: `${job}s`,
      people: grouped.get(job)!,
    }));

  if (crew.length === 0) return null;

  const visible = pickCrew(crew, limit);
  if (visible.length === 0) return null;

  return (
    <div>
      <h2 className="mb-4 text-xl font-semibold">Crew</h2>
      <dl className="glass-subtle grid grid-cols-[auto_1fr] gap-x-6 gap-y-3 rounded-xl px-5 py-4 text-sm">
        {entries.map(({ job, people }) => (
          <div key={job} className="contents">
            <dt className="text-muted-foreground text-[0.8125rem] font-medium">{job}</dt>
            <dd className="text-foreground/85 truncate">
              {people.map((p, i) => (
                <span key={p.name}>
                  {i > 0 && ", "}
                  {p.personId ? (
                    <ViewTransitionLink
                      to={buildPersonCatalogHref(p.personId)}
                      className="hover:text-primary transition-colors"
                    >
                      {p.name}
                    </ViewTransitionLink>
                  ) : (
                    p.name
                  )}
                </span>
              ))}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
