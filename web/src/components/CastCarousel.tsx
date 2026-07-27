import { ChevronLeft, ChevronRight } from "lucide-react";
import ViewTransitionLink from "@/components/ViewTransitionLink";
import type { CastMember } from "@/api/types";
import { useCarouselEmbla } from "@/hooks/useCarouselEmbla";
import { buildPersonCatalogHref } from "@/pages/catalogSearchParams";
import { getInitials } from "@/lib/text";
import { cn } from "@/lib/utils";

/** Shared portrait card width — keep in sync with CrewList. */
export const PERSON_CARD_WIDTH_CLASS = "w-[160px]";

interface CastCarouselProps {
  cast: CastMember[];
  limit?: number;
  /**
   * When true, the carousel adds MediaCarousel-style edge padding so it can sit
   * at the top level of a page (outside `.page-shell`) and align with other
   * full-bleed rows.
   */
  fullBleed?: boolean;
}

export default function CastCarousel({ cast, limit = 20, fullBleed = false }: CastCarouselProps) {
  const { emblaRef, canScrollPrev, canScrollNext, scrollPrev, scrollNext } = useCarouselEmbla();

  if (cast.length === 0) return null;

  const visible = cast
    .slice()
    .sort((a, b) => a.order - b.order)
    .slice(0, limit);

  return (
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
              <li key={`${member.name}-${member.order}`} className="embla__slide shrink-0">
                <PersonCard
                  name={member.name}
                  subtitle={member.character}
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
  );
}

export function PersonCard({
  name,
  subtitle,
  photoUrl,
  href,
}: {
  name: string;
  subtitle?: string | null;
  photoUrl?: string | null;
  href: string | null;
}) {
  const inner = (
    <>
      <div className="media-card-image mb-2.5 aspect-[2/3] overflow-hidden rounded-lg">
        {photoUrl ? (
          <img
            src={photoUrl}
            alt={name}
            className="h-full w-full object-cover transition-transform duration-300 group-hover/person:scale-105"
            loading="lazy"
          />
        ) : (
          <div className="bg-surface text-muted-foreground flex h-full w-full items-center justify-center text-lg font-semibold">
            {getInitials(name)}
          </div>
        )}
      </div>
      <div className="px-0.5">
        <div className="text-foreground truncate text-sm font-medium">{name}</div>
        {subtitle ? <div className="text-muted-foreground truncate text-xs">{subtitle}</div> : null}
      </div>
    </>
  );

  if (href) {
    return (
      <ViewTransitionLink to={href} className={cn("group/person block", PERSON_CARD_WIDTH_CLASS)}>
        {inner}
      </ViewTransitionLink>
    );
  }
  return <div className={cn("group/person", PERSON_CARD_WIDTH_CLASS)}>{inner}</div>;
}
