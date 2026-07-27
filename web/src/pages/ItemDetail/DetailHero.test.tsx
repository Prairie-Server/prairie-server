import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";

import { resetImageFormatsCacheForTests } from "@/lib/imageFormats";
import DetailHero from "./DetailHero";

describe("DetailHero artwork revisions", () => {
  beforeEach(() => {
    resetImageFormatsCacheForTests();
    localStorage.setItem("prairie.imageFormats", "avif,webp,png");
  });

  it("treats a changed poster URL as unloaded until that revision finishes loading", () => {
    const { rerender } = render(<DetailHero title="Blade Runner" posterUrl="/poster.rev-a.webp" />);

    const first = screen.getByRole("img", { name: "Blade Runner" });
    expect(first).toHaveClass("opacity-0");
    fireEvent.load(first);
    expect(first).toHaveClass("opacity-100");

    rerender(<DetailHero title="Blade Runner" posterUrl="/poster.rev-b.webp" />);

    const replacement = screen.getByRole("img", { name: "Blade Runner" });
    // ArtworkImage prefers the best detected format sibling of a WebP poster URL.
    expect(replacement).toHaveAttribute("src", "/poster.rev-b.avif");
    expect(replacement).toHaveClass("opacity-0");
    fireEvent.load(replacement);
    expect(replacement).toHaveClass("opacity-100");
  });
});
