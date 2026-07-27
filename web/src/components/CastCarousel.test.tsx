import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import CastCarousel from "./CastCarousel";

describe("CastCarousel", () => {
  it("renders an Embla carousel with cast cards", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter initialEntries={["/item/series-1"]}>
        <CastCarousel
          cast={[
            {
              name: "Winona Ryder",
              character: "Joyce Byers",
              order: 1,
              person_id: "person-001",
              photo_url: "https://images.example.test/winona.jpg",
              photo_thumbhash: "thumbhash-cast",
            },
            {
              name: "David Harbour",
              character: "Jim Hopper",
              order: 2,
              person_id: "person-002",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(markup).toContain("embla__viewport");
    expect(markup).toContain("embla__container");
    expect(markup).toContain("w-[160px]");
    expect(markup).toContain('href="/person/person-001"');
    expect(markup).toContain('src="https://images.example.test/winona.jpg"');
    expect(markup).toContain(">DH<");
    expect(markup).not.toContain('data-slot="scroll-area"');
  });
});
