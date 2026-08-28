import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { PictureImage } from "./PictureImage";

describe("PictureImage", () => {
  it("renders AVIF → WebP → PNG sources for raster siblings", () => {
    const { container } = render(
      <PictureImage
        src="/prairie-icon-1024.png"
        alt="Prairie"
        className="h-8 w-8"
      />,
    );
    const picture = container.querySelector("picture");
    expect(picture).toBeTruthy();
    const sources = Array.from(container.querySelectorAll("source"));
    expect(
      sources.map((s) => [s.getAttribute("type"), s.getAttribute("srcset")]),
    ).toEqual([
      ["image/avif", "/prairie-icon-1024.avif"],
      ["image/webp", "/prairie-icon-1024.webp"],
    ]);
    const img = container.querySelector("img");
    expect(img).toHaveAttribute("src", "/prairie-icon-1024.png");
    expect(img).toHaveAttribute("alt", "Prairie");
    expect(img).toHaveClass("h-8", "w-8");
  });

  it("falls back to a plain img for non-raster paths", () => {
    const { container } = render(<PictureImage src="/mark.svg" alt="Mark" />);
    expect(container.querySelector("picture")).toBeNull();
    expect(container.querySelector("img")).toHaveAttribute("src", "/mark.svg");
  });
});
