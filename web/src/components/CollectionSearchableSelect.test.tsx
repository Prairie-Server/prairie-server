import { renderToStaticMarkup } from "react-dom/server";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";

vi.mock("radix-ui", async () => {
  return {
    Popover: {
      Root: ({ children, modal }: { children: ReactNode; modal?: boolean }) => (
        <div data-modal={String(modal)}>{children}</div>
      ),
      Trigger: ({ children }: { children: ReactNode }) => <>{children}</>,
      Portal: ({ children }: { children: ReactNode }) => <>{children}</>,
      Content: ({ children }: { children: ReactNode }) => <div>{children}</div>,
    },
  };
});

import { CollectionSearchableSelect } from "./CollectionSearchableSelect";

describe("CollectionSearchableSelect", () => {
  it("uses a non-modal popover so wheel scrolling works inside modal sheets", () => {
    const markup = renderToStaticMarkup(
      <CollectionSearchableSelect
        options={[
          { id: "col-1", title: "Trending", group: "Movies", source: "user" },
          {
            id: "col-2",
            title: "Weekly",
            group: "TV Shows",
            source: "library",
          },
        ]}
        value=""
        onChange={() => {}}
      />,
    );

    expect(markup).toContain('data-modal="false"');
  });
});
