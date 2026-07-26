import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ArtworkImage } from "./ArtworkImage";

describe("ArtworkImage", () => {
  it("renders nothing without a src", () => {
    const { container } = render(<ArtworkImage src={null} alt="x" />);
    expect(container.querySelector("img")).toBeNull();
  });

  it("prefers the AVIF sibling for WebP artwork", () => {
    render(<ArtworkImage src="/art/original.rev.webp" alt="Poster" />);
    expect(screen.getByRole("img", { name: "Poster" })).toHaveAttribute(
      "src",
      "/art/original.rev.avif",
    );
  });

  it("falls back to WebP when AVIF fails to load", () => {
    render(<ArtworkImage src="/art/original.rev.webp" alt="Poster" />);
    const img = screen.getByRole("img", { name: "Poster" });
    fireEvent.error(img);
    expect(img).toHaveAttribute("src", "/art/original.rev.webp");
  });

  it("falls back to PNG when WebP also fails", () => {
    render(<ArtworkImage src="/art/original.rev.webp" alt="Poster" />);
    const img = screen.getByRole("img", { name: "Poster" });
    fireEvent.error(img);
    fireEvent.error(img);
    expect(img).toHaveAttribute("src", "/art/original.rev.png");
  });

  it("uses the original src when it is not WebP", () => {
    render(<ArtworkImage src="/art/cover.jpg" alt="Cover" />);
    expect(screen.getByRole("img", { name: "Cover" })).toHaveAttribute("src", "/art/cover.jpg");
  });

  it("forwards onLoad", () => {
    const onLoad = vi.fn();
    render(<ArtworkImage src="/art/original.webp" alt="Poster" onLoad={onLoad} />);
    fireEvent.load(screen.getByRole("img", { name: "Poster" }));
    expect(onLoad).toHaveBeenCalledOnce();
  });

  it("forwards onError only after all candidates fail", () => {
    const onError = vi.fn();
    render(<ArtworkImage src="/art/original.rev.webp" alt="Poster" onError={onError} />);
    const img = screen.getByRole("img", { name: "Poster" });
    fireEvent.error(img);
    fireEvent.error(img);
    expect(onError).not.toHaveBeenCalled();
    fireEvent.error(img);
    expect(onError).toHaveBeenCalledOnce();
  });
});
