/* eslint-disable react-hooks/set-state-in-effect */
import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { LayoutDashboard } from "lucide-react";
import HeroBanner from "@/components/HeroBanner";
import SectionRow from "@/components/SectionRow";
import TasteSeedBanner from "@/components/TasteSeedBanner";
import { Skeleton } from "@/components/ui/skeleton";
import type { HomeSectionItemsResponse, ResolvedSection } from "@/api/types";
import { useDocumentTitle } from "@/hooks/useDocumentTitle";
import { HERO_BANNER_SIZE, HOME_BRAND_HERO_SIZE } from "@/lib/design-system";
import { PrairieBrand } from "@/components/PrairieBrand";
import { useServerBranding } from "@/hooks/useServerBranding";
import { sectionKeys } from "@/hooks/queries/keys";
import { fetchHomeSectionItems, useHomeLayout } from "@/hooks/queries/sections";
import { planNextHomeSectionBatch } from "./homeSectionQueue";
import { buildHomeSectionViewModel, type HomeSectionSlot } from "./homeSectionState";
import { collectCachedHomeSections } from "./homeSectionCache";

const SECTION_STALE_TIME = 5 * 60 * 1000;
const MAX_CONCURRENT_SECTION_REQUESTS = 5;
const SKELETON_CARD_COUNT = 7;

function useHomeRefreshSignal() {
  return useQuery({
    queryKey: sectionKeys.homeRefreshSignal(),
    queryFn: () => 0,
    initialData: 0,
    staleTime: Number.POSITIVE_INFINITY,
    gcTime: Number.POSITIVE_INFINITY,
  });
}

export default function Home() {
  const queryClient = useQueryClient();
  const { data, isLoading, isError, refetch } = useHomeLayout();
  const { data: homeRefreshSignal = 0 } = useHomeRefreshSignal();
  const [loadedSections, setLoadedSections] = useState<Map<string, ResolvedSection>>(new Map());
  const [failedIds, setFailedIds] = useState<Set<string>>(new Set());
  const [inFlightIds, setInFlightIds] = useState<Set<string>>(new Set());
  const [completedIds, setCompletedIds] = useState<Set<string>>(new Set());
  const activeSectionIdsRef = useRef<Set<string>>(new Set());

  useDocumentTitle("Home");

  const layout = useMemo(() => data?.sections ?? [], [data?.sections]);
  const layoutResetKey = layout
    .map(
      (section) =>
        `${section.id}:${section.section_type}:${section.title}:${section.featured ? "featured" : "row"}:${section.item_limit}:${section.is_custom ? "custom" : "default"}:${section.customized ? "customized" : "clean"}`,
    )
    .join("|");

  useEffect(() => {
    const activeIds = layout.map((section) => section.id);
    activeSectionIdsRef.current = new Set(activeIds);
    setLoadedSections(
      collectCachedHomeSections(layout, (sectionId) =>
        queryClient.getQueryData<HomeSectionItemsResponse>(sectionKeys.homeItems(sectionId)),
      ),
    );
    setFailedIds(new Set());
    setInFlightIds(new Set());
    setCompletedIds(new Set());

    return () => {
      activeSectionIdsRef.current = new Set();
      activeIds.forEach((sectionId) => {
        void queryClient.cancelQueries({ queryKey: sectionKeys.homeItems(sectionId) });
      });
    };
  }, [homeRefreshSignal, layout, layoutResetKey, queryClient]);

  useEffect(() => {
    if (layout.length === 0) return;

    const nextIds = planNextHomeSectionBatch({
      layout,
      loadedIds: completedIds,
      inFlightIds,
      maxConcurrentRequests: MAX_CONCURRENT_SECTION_REQUESTS,
    });

    if (nextIds.length === 0) return;

    setInFlightIds((prev) => {
      const next = new Set(prev);
      nextIds.forEach((id) => next.add(id));
      return next;
    });

    nextIds.forEach((sectionId) => {
      void queryClient
        .fetchQuery<HomeSectionItemsResponse>({
          queryKey: sectionKeys.homeItems(sectionId),
          queryFn: ({ signal }) => fetchHomeSectionItems(sectionId, { signal }),
          staleTime: SECTION_STALE_TIME,
        })
        .then((response) => {
          if (!activeSectionIdsRef.current.has(sectionId)) return;

          setLoadedSections((prev) => {
            const next = new Map(prev);
            next.set(sectionId, response.section);
            return next;
          });
          setFailedIds((prev) => {
            if (!prev.has(sectionId)) return prev;
            const next = new Set(prev);
            next.delete(sectionId);
            return next;
          });
          setCompletedIds((prev) => {
            const next = new Set(prev);
            next.add(sectionId);
            return next;
          });
        })
        .catch(() => {
          if (!activeSectionIdsRef.current.has(sectionId)) return;

          setFailedIds((prev) => {
            const next = new Set(prev);
            next.add(sectionId);
            return next;
          });
          setCompletedIds((prev) => {
            const next = new Set(prev);
            next.add(sectionId);
            return next;
          });
        })
        .finally(() => {
          if (!activeSectionIdsRef.current.has(sectionId)) return;

          setInFlightIds((prev) => {
            if (!prev.has(sectionId)) return prev;
            const next = new Set(prev);
            next.delete(sectionId);
            return next;
          });
        });
    });
  }, [completedIds, inFlightIds, layout, queryClient]);

  const viewModel = buildHomeSectionViewModel({
    layout,
    loadedSections,
    failedIds,
  });

  const retrySection = (sectionId: string) => {
    queryClient.removeQueries({ queryKey: sectionKeys.homeItems(sectionId) });
    setFailedIds((prev) => {
      if (!prev.has(sectionId)) return prev;
      const next = new Set(prev);
      next.delete(sectionId);
      return next;
    });
    setCompletedIds((prev) => {
      if (!prev.has(sectionId)) return prev;
      const next = new Set(prev);
      next.delete(sectionId);
      return next;
    });
  };
  const heroSlot = renderHeroSlot(viewModel.hero, retrySection);
  const hasHeroSlot = heroSlot !== null;

  if (isLoading && !data) {
    return <HomePageSkeleton />;
  }

  if (isError && !data) {
    return (
      <div className="page-shell flex min-h-[50vh] items-center justify-center py-12">
        <div className="surface-panel max-w-md rounded-[1.8rem] border-0 p-6 text-center shadow-none">
          <p className="text-lg font-semibold">Unable to load the homepage</p>
          <p className="text-muted-foreground mt-2 text-sm">
            We couldn&apos;t load your section layout right now.
          </p>
          <button
            type="button"
            onClick={() => void refetch()}
            className="border-border hover:bg-muted/40 mt-4 rounded-lg border px-4 py-2 text-sm font-medium"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  return (
    <>
      <h1 className="sr-only">Home</h1>
      <div className={`home-sections space-y-10 ${hasHeroSlot ? "pb-2" : "pb-2"}`}>
        {hasHeroSlot ? heroSlot : <HomeBrandHero />}
        <TasteSeedBanner />

        {viewModel.rows.map((slot) => {
          if (slot.state === "empty") {
            return null;
          }
          if (slot.state === "ready" && slot.section) {
            return (
              <div key={slot.layout.id} className="section-fade-in">
                <SectionRow section={slot.section} />
              </div>
            );
          }
          if (slot.state === "error") {
            return (
              <div key={slot.layout.id} className="section-fade-in">
                <SectionErrorRow
                  title={slot.layout.title}
                  onRetry={() => retrySection(slot.layout.id)}
                />
              </div>
            );
          }
          return (
            <div key={slot.layout.id} className="section-fade-in">
              <SectionLoadingRow title={slot.layout.title} />
            </div>
          );
        })}

        {layout.length === 0 && !isLoading && (
          <div className="section-fade-in surface-panel flex h-64 flex-col items-center justify-center gap-3 rounded-[1.8rem] border-0 px-6 text-center">
            <LayoutDashboard className="text-muted-foreground h-10 w-10" />
            <p className="text-muted-foreground text-sm">
              Your home screen is empty. Customize it by adding sections to display your media.
            </p>
            <Link to="/settings/home" className="text-primary text-sm font-medium hover:underline">
              Customize Home Screen
            </Link>
          </div>
        )}
      </div>
    </>
  );
}

function HomeBrandHero() {
  const { serverName } = useServerBranding();

  return (
    <section
      className={`home-hero border-border/60 relative mb-10 flex w-full items-end overflow-hidden border-b ${HOME_BRAND_HERO_SIZE}`}
      aria-label="Welcome"
    >
      <div
        aria-hidden
        className="pointer-events-none absolute inset-0"
        style={{
          background: `
            radial-gradient(ellipse 90% 70% at 50% 0%, color-mix(in srgb, var(--primary) 20%, transparent), transparent 58%),
            radial-gradient(circle at 18% 80%, color-mix(in srgb, var(--ambient) 14%, transparent), transparent 36%),
            linear-gradient(165deg, var(--background), color-mix(in srgb, var(--background) 88%, #1a1610))
          `,
        }}
      />
      <div className="hero-gradient" />
      <div className="relative z-10 flex w-full flex-col items-start gap-4 px-4 pb-12 sm:px-6 sm:pb-14 lg:px-10 lg:pb-16 xl:px-12">
        <PrairieBrand className="brand-reveal h-14 w-[148px] sm:h-16 sm:w-[168px]" />
        <div className="auth-brand-copy max-w-xl space-y-2">
          <p className="font-display text-3xl font-bold tracking-[-0.03em] sm:text-4xl lg:text-5xl">
            {serverName}
          </p>
          <p className="text-muted-foreground max-w-md text-sm leading-6 sm:text-base">
            Your library, ready when you are.
          </p>
        </div>
      </div>
    </section>
  );
}

function renderHeroSlot(hero: HomeSectionSlot | null, retrySection: (sectionId: string) => void) {
  if (!hero) return null;

  if (hero.state === "ready" && hero.section) {
    return <HeroBanner items={hero.section.items} maxSlides={hero.layout.item_limit} />;
  }

  if (hero.state === "error") {
    return (
      <div className={`bg-muted relative ${HERO_BANNER_SIZE} w-full overflow-hidden`}>
        <div className="from-background via-background/70 absolute inset-0 bg-gradient-to-t to-transparent" />
        <div className="relative flex h-full items-end px-4 pb-10 sm:px-6 sm:pb-12 lg:px-12 lg:pb-16">
          <div className="max-w-xl space-y-3">
            <p className="text-3xl font-bold">{hero.layout.title}</p>
            <p className="text-muted-foreground text-sm">
              This featured section could not be loaded right now.
            </p>
            <button
              type="button"
              onClick={() => retrySection(hero.layout.id)}
              className="border-border hover:bg-muted/40 rounded-lg border px-4 py-2 text-sm font-medium"
            >
              Retry section
            </button>
          </div>
        </div>
      </div>
    );
  }

  if (hero.state === "empty") {
    return null;
  }

  return <Skeleton className={`${HERO_BANNER_SIZE} w-full rounded-none`} />;
}

function HomePageSkeleton() {
  return (
    <div className="home-sections space-y-8">
      <Skeleton className={`${HERO_BANNER_SIZE} w-full rounded-none`} />
      {Array.from({ length: 3 }).map((_, i) => (
        <div key={i} className="section-fade-in space-y-3 px-4 sm:px-6 lg:px-12">
          <Skeleton className="h-6 w-48" />
          <div className="flex gap-4">
            {Array.from({ length: SKELETON_CARD_COUNT }).map((_, j) => (
              <Skeleton
                key={j}
                className="aspect-[2/3] w-[130px] shrink-0 rounded-lg sm:w-[150px] lg:w-[178px]"
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}

function SectionLoadingRow({ title }: { title: string }) {
  return (
    <section className="space-y-3 px-4 sm:px-6 lg:px-12">
      <h2 className="text-foreground h-6 text-sm font-semibold">{title}</h2>
      <div className="flex gap-4 overflow-hidden">
        {Array.from({ length: SKELETON_CARD_COUNT }).map((_, index) => (
          <Skeleton
            key={index}
            className="aspect-[2/3] w-[130px] shrink-0 rounded-lg sm:w-[150px] lg:w-[178px]"
          />
        ))}
      </div>
    </section>
  );
}

function SectionErrorRow({ title, onRetry }: { title: string; onRetry: () => void }) {
  return (
    <section className="space-y-3 px-4 sm:px-6 lg:px-12">
      <h2 className="text-foreground h-6 text-sm font-semibold">{title}</h2>
      <div className="surface-panel flex items-center justify-between rounded-[1.4rem] border-0 px-5 py-4">
        <p className="text-muted-foreground text-sm">This section could not be loaded right now.</p>
        <button
          type="button"
          onClick={onRetry}
          className="border-border hover:bg-muted/40 rounded-lg border px-4 py-2 text-sm font-medium"
        >
          Retry
        </button>
      </div>
    </section>
  );
}
