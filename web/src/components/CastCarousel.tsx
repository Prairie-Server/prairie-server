import { Fragment, memo, useMemo, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import ViewTransitionLink from "@/components/ViewTransitionLink";
import type { CastMember } from "@/api/types";
import { CREW_ROLE_CAPTION, type CrewGroup } from "@/components/castCrewGroups";
import { useT } from "@/i18n";
import { usePrefetchPeople } from "@/hooks/queries/people";
import { useCarouselEmbla } from "@/hooks/useCarouselEmbla";
import { buildPersonCatalogHref } from "@/pages/catalogSearchParams";
import { artworkSrcSet, PROFILE_WIDTHS } from "@/lib/artworkUrl";
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
  /** Warm person detail for the shown cast so opening one renders at once. */
  prefetchPeople?: boolean;
  /**
   * Crew groups shown ahead of the cast, in order (e.g. Director, then
   * Writers). Each group after the first, and the cast, gets a labelled
   * divider so the row reads as one "Cast & Crew" strip.
   */
  crewGroups?: CrewGroup[];
}

interface CreditCardData {
  name: string;
  subtitle: string;
  personId: string;
  photoUrl?: string;
}

interface CreditSection {
  key: string;
  label: string;
  cards: CreditCardData[];
}

function CastCarousel({
  cast,
  limit = 20,
  fullBleed = false,
  prefetchPeople = false,
  crewGroups,
}: CastCarouselProps) {
  const t = useT();
  const { emblaRef, canScrollPrev, canScrollNext, scrollPrev, scrollNext } = useCarouselEmbla();
  const visible = useMemo(
    () =>
      cast
        .slice()
        .sort((a, b) => a.order - b.order)
        .slice(0, limit),
    [cast, limit],
  );
  const sections = useMemo(() => {
    const out: CreditSection[] = [];
    for (const group of crewGroups ?? []) {
      const seen = new Set<string>();
      const cards: CreditCardData[] = [];
      for (const member of group.members) {
        const key = member.person_id || member.name;
        if (seen.has(key)) continue;
        seen.add(key);
        cards.push({
          name: member.name,
          subtitle: t(CREW_ROLE_CAPTION[group.role]),
          personId: member.person_id,
          photoUrl: member.photo_url,
        });
        if (cards.length === group.max) break;
      }
      if (cards.length > 0) {
        out.push({ key: group.label, label: t(group.label), cards });
      }
    }
    if (visible.length > 0) {
      out.push({
        key: "cast",
        label: t("web.detail.castCrew.cast"),
        cards: visible.map((member) => ({
          name: member.name,
          subtitle: member.character,
          personId: member.person_id,
          photoUrl: member.photo_url,
        })),
      });
    }
    return out;
  }, [crewGroups, visible, t]);
  const prefetchIds = useMemo(
    () =>
      sections.flatMap((section) => section.cards.flatMap((c) => (c.personId ? [c.personId] : []))),
    [sections],
  );

  if (sections.length === 0) return null;

  return (
    <div className="group/carousel relative">
      {prefetchPeople && <PrefetchCastPeople personIds={prefetchIds} />}
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
          {sections.map((section, sectionIndex) => (
            <Fragment key={section.key}>
              {sectionIndex > 0 && (
                <li
                  aria-hidden="true"
                  className="embla__slide flex shrink-0 items-start self-stretch px-3 pb-10"
                >
                  <div className="flex h-full items-center gap-2">
                    <span className="text-muted-foreground/70 rotate-180 text-[0.625rem] font-semibold tracking-[0.2em] uppercase [text-orientation:sideways] [writing-mode:vertical-rl]">
                      {section.label}
                    </span>
                    <span className="bg-border h-full w-px" />
                  </div>
                </li>
              )}
              {section.cards.map((card, i) => (
                <li
                  key={`${section.key}-${card.personId || card.name}-${i}`}
                  className="embla__slide shrink-0"
                >
                  <PersonCard
                    name={card.name}
                    subtitle={card.subtitle}
                    photoUrl={card.photoUrl}
                    href={card.personId ? buildPersonCatalogHref(card.personId) : null}
                  />
                </li>
              ))}
            </Fragment>
          ))}
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
  );
}

export default memo(CastCarousel);

function PrefetchCastPeople({ personIds }: { personIds: string[] }) {
  usePrefetchPeople(personIds);
  return null;
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
  // A non-artwork URL (third-party image, or a path with no variant segment)
  // yields no srcSet, and `sizes` without one is meaningless — mirrors what
  // ArtworkImage does for the same reason.
  const photoSrcSet = artworkSrcSet(photoUrl, PROFILE_WIDTHS);
  // Portrait art is the flakiest kind: an expired signature, a rung the store
  // never generated, or a provider 404 all end as a broken-image glyph in a
  // face-shaped box. Initials are a better answer than a torn page.
  //
  // Keyed to the URL that failed, not a boolean: carousel slides are reused as
  // the cast list changes, and a re-signed URL arrives on the same instance. A
  // sticky flag would strand a person on initials until the tree remounted.
  const [failedPhotoUrl, setFailedPhotoUrl] = useState<string | null>(null);
  const showPhoto = Boolean(photoUrl) && failedPhotoUrl !== photoUrl;
  const inner = (
    <>
      <div className="media-card-image mb-2.5 aspect-[2/3] overflow-hidden rounded-lg">
        {showPhoto ? (
          <img
            src={photoUrl ?? undefined}
            srcSet={photoSrcSet || undefined}
            sizes={photoSrcSet ? "160px" : undefined}
            alt={name}
            className="h-full w-full object-cover transition-transform duration-300 group-hover/person:scale-105"
            loading="lazy"
            onError={() => setFailedPhotoUrl(photoUrl ?? null)}
            decoding="async"
          />
        ) : (
          <div className="bg-surface text-muted-foreground flex h-full w-full items-center justify-center text-lg font-semibold">
            {getInitials(name)}
          </div>
        )}
      </div>
      <div className="px-0.5">
        <div className="bidi-text text-foreground truncate text-sm font-medium">{name}</div>
        {subtitle ? (
          <div className="bidi-text text-muted-foreground truncate text-xs">{subtitle}</div>
        ) : null}
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
