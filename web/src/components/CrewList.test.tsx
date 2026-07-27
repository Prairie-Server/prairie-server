import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it } from "vitest";
import CrewList from "./CrewList";

describe("CrewList", () => {
  it("renders a portrait carousel matching cast card metrics", () => {
    const markup = renderToStaticMarkup(
      <MemoryRouter initialEntries={["/item/movie-1"]}>
        <CrewList
          crew={[
            {
              name: "Denis Villeneuve",
              job: "Director",
              person_id: "person-dir",
              photo_url: "https://images.example.test/denis.jpg",
            },
            {
              name: "Jon Spaihts",
              job: "Writer",
              person_id: "person-writer",
            },
            {
              name: "Cut Room",
              job: "Editor",
              person_id: "person-editor",
            },
          ]}
        />
      </MemoryRouter>,
    );

    expect(markup).toContain(">Crew<");
    expect(markup).toContain("embla__viewport");
    expect(markup).toContain("w-[160px]");
    expect(markup).toContain('href="/person/person-dir"');
    expect(markup).toContain('src="https://images.example.test/denis.jpg"');
    expect(markup).toContain(">Director<");
    expect(markup).toContain(">Writer<");
    expect(markup).not.toContain(">Editor<");
    expect(markup).not.toContain("glass-subtle");
  });
});
