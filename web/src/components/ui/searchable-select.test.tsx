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

import { SearchableMultiSelect, SearchableSelect } from "./searchable-select";

describe("searchable select popovers", () => {
  it("render as non-modal popovers so nested dialog and sheet scrolling still works", () => {
    const singleMarkup = renderToStaticMarkup(
      <SearchableSelect options={["Movies", "Shows"]} value="" onChange={() => {}} />,
    );
    const multiMarkup = renderToStaticMarkup(
      <SearchableMultiSelect options={["Movies", "Shows"]} value={[]} onChange={() => {}} />,
    );

    expect(singleMarkup).toContain('data-modal="false"');
    expect(multiMarkup).toContain('data-modal="false"');
  });
});
