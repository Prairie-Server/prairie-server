// @vitest-environment jsdom

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { PersonCard } from "./CastCarousel";

const URL_A = "/artwork/people/p1/profile/w500.7.webp?sig=a&expires=1";
const URL_B = "/artwork/people/p1/profile/w500.8.webp?sig=b&expires=2";

describe("PersonCard portrait failure", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(async () => {
    await act(async () => {
      root.unmount();
    });
    container.remove();
  });

  async function render(photoUrl: string) {
    await act(async () => {
      root.render(
        <MemoryRouter>
          <PersonCard name="Winona Ryder" subtitle="Joyce Byers" photoUrl={photoUrl} href={null} />
        </MemoryRouter>,
      );
    });
  }

  function image() {
    return container.querySelector("img");
  }

  it("falls back to initials when the portrait fails to load", async () => {
    await render(URL_A);
    expect(image()).not.toBeNull();

    await act(async () => {
      image()?.dispatchEvent(new Event("error"));
    });

    expect(image()).toBeNull();
    expect(container.textContent).toContain("WR");
  });

  // A re-signed URL arrives on the same carousel slide, so a sticky boolean
  // would strand this person on initials until the tree remounted.
  it("recovers when a different URL arrives after a failure", async () => {
    await render(URL_A);
    await act(async () => {
      image()?.dispatchEvent(new Event("error"));
    });
    expect(image()).toBeNull();

    await render(URL_B);

    const recovered = image();
    expect(recovered).not.toBeNull();
    expect(recovered?.getAttribute("src")).toBe(URL_B);
  });

  it("keeps showing initials while the failed URL is still the current one", async () => {
    await render(URL_A);
    await act(async () => {
      image()?.dispatchEvent(new Event("error"));
    });
    await render(URL_A);
    expect(image()).toBeNull();
    expect(container.textContent).toContain("WR");
  });
});
