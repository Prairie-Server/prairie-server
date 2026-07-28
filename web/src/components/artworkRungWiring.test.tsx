import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";
import CastCarousel from "./CastCarousel";
import EpisodeRow from "./EpisodeRow";
import { PROFILE_WIDTHS, STILL_WIDTHS } from "@/lib/artworkUrl";

vi.mock("@/components/overlays/CardOverlays", () => ({ default: () => null }));
vi.mock("@/hooks/useOverlayPrefs", () => ({ useOverlayPrefs: () => ({ prefs: null }) }));

const STILL_URL = "/artwork/tv/series-1/ep-1/still/w500.4.webp?sig=abc&expires=99";
const PHOTO_URL = "/artwork/people/person-1/profile/w500.2.webp?sig=abc&expires=99";

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

  // The ladders mirror artworkkey.VariantWidths; drifting apart means the
  // browser requests rungs the server never generated.
  it("keeps the ladders in step with the server", () => {
    expect([...STILL_WIDTHS]).toEqual([300, 500]);
    expect([...PROFILE_WIDTHS]).toEqual([200, 300, 500]);
  });
});
