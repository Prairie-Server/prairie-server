import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import CastCarousel from "./CastCarousel";
import EpisodeRow from "./EpisodeRow";
import ContinueWatchingCard from "./ContinueWatchingCard";
import { PROFILE_WIDTHS, STILL_WIDTHS } from "@/lib/artworkUrl";

vi.mock("@/components/overlays/CardOverlays", () => ({ default: () => null }));
vi.mock("@/hooks/useOverlayPrefs", () => ({ useOverlayPrefs: () => ({ prefs: null }) }));
// ContinueWatchingCard reaches for the playback controller, which only exists
// under the watch host. These tests render markup to inspect srcSet rungs, so a
// no-op controller is enough.
// The row's overflow menu pulls in admin/auth hooks that need providers these
// markup-only tests do not stand up.
vi.mock("@/components/MediaItemMenu", () => ({ default: () => null }));
vi.mock("@/playback/watchPlaybackContext", () => ({
  useWatchPlaybackController: () => ({ startPlayback: () => {} }),
}));

const STILL_URL = "/artwork/tv/series-1/ep-1/still/w500.4.webp?sig=abc&expires=99";
const PHOTO_URL = "/artwork/people/person-1/profile/w500.2.webp?sig=abc&expires=99";
const POSTER_URL = "/artwork/tmdb/movie/856/poster/w500.7.webp?sig=abc&expires=99";

/**
 * These images used to be plain <img src> tags pinned to whatever rung the
 * server signed — w500 for a 140px still and a 160px portrait. The ladders only
 * pay off if the call sites actually declare them.
 */
describe("artwork rung wiring", () => {
  it("offers the still ladder to a 140px episode thumbnail", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <EpisodeRow
          episode={{
            content_id: "ep-001",
            season_number: 1,
            episode_number: 1,
            title: "Pilot",
            overview: "",
            air_date: "2024-01-01",
            runtime: 42,
            still_url: STILL_URL,
            still_thumbhash: "",
            files: [],
          }}
        />
      </MemoryRouter>,
    );

    expect(markup).toContain('sizes="140px"');
    expect(markup).toContain("/w300.4.webp");
    expect(markup).toContain("300w");
    expect(markup).toContain("500w");
    // Stills have no w200 rung: 140px at 2x needs 280, and television stills
    // render ~358px, so w200 would upscale on the only surfaces showing it.
    expect(markup).not.toContain("/w200.");
  });

  it("offers the portrait ladder to a 160px cast card", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter initialEntries={["/item/series-1"]}>
        <CastCarousel
          cast={[
            {
              name: "Winona Ryder",
              character: "Joyce Byers",
              order: 1,
              person_id: "person-001",
              photo_url: PHOTO_URL,
              photo_thumbhash: "",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(markup).toContain('sizes="160px"');
    expect(markup).toContain("/w200.2.webp");
    expect(markup).toContain("200w");
    expect(markup).toContain("300w");
    expect(markup).toContain("500w");
  });

  // A third-party or non-artwork URL has no variant segment to rewrite, so the
  // markup must fall back to a bare src rather than emitting a broken srcSet.
  it("emits no srcSet for a URL that is not a cached artwork key", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter initialEntries={["/item/series-1"]}>
        <CastCarousel
          cast={[
            {
              name: "David Harbour",
              character: "Jim Hopper",
              order: 1,
              person_id: "person-002",
              photo_url: "https://images.example.test/harbour.jpg",
              photo_thumbhash: "",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(markup).toContain('src="https://images.example.test/harbour.jpg"');
    expect(markup).not.toContain("srcSet");
    expect(markup).not.toContain("sizes=");
  });

  // A Continue Watching episode shows its still, which has no rung above w500.
  // Offering the backdrop ladder here made the browser choose w1280 and 404, so
  // the card rendered its placeholder -- while the same episode looked right on
  // clients that fetch the canonical URL with no srcSet.
  it("offers the still ladder to a continue-watching episode", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <ContinueWatchingCard
          sectionItem={
            {
              content_id: "ep-001",
              type: "episode",
              title: "Pilot",
              series_title: "Severance",
              season_number: 2,
              episode_number: 7,
              backdrop_url: STILL_URL,
            } as never
          }
        />
      </MemoryRouter>,
    );

    expect(markup).toContain("300w");
    expect(markup).toContain("500w");
    // The rungs the server never generated for a still.
    expect(markup).not.toContain("1280w");
    expect(markup).not.toContain("1920w");
    expect(markup).not.toContain("/w1280.");
    expect(markup).not.toContain("/w1920.");
  });

  // A movie keeps the backdrop ladder: this is the case that already worked, and
  // narrowing every card to the still rungs would regress it.
  it("keeps the backdrop ladder for a continue-watching movie", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <ContinueWatchingCard
          sectionItem={
            {
              content_id: "movie-856",
              type: "movie",
              title: "Arrival",
              backdrop_url: "/artwork/tmdb/movie/856/backdrop/w1280.7.webp?sig=abc&expires=99",
              poster_url: POSTER_URL,
            } as never
          }
        />
      </MemoryRouter>,
    );

    expect(markup).toContain("1280w");
  });

  // The ladders mirror artworkkey.VariantWidths; drifting apart means the
  // browser requests rungs the server never generated.
  it("keeps the ladders in step with the server", () => {
    expect([...STILL_WIDTHS]).toEqual([300, 500]);
    expect([...PROFILE_WIDTHS]).toEqual([200, 300, 500]);
  });
});
