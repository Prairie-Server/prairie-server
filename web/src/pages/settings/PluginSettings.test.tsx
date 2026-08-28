import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { describe, expect, it, vi } from "vitest";

import PluginSettings from "./PluginSettings";

const usePluginSettingsListMock = vi.fn();
const usePluginSettingsDetailMock = vi.fn();
const useUpdatePluginSettingsMock = vi.fn();

vi.mock("@/hooks/queries/pluginSettings", () => ({
  usePluginSettingsList: () => usePluginSettingsListMock(),
  usePluginSettingsDetail: (...args: unknown[]) =>
    usePluginSettingsDetailMock(...args),
  useUpdatePluginSettings: () => useUpdatePluginSettingsMock(),
}));

describe("PluginsSettings user page", () => {
  it("renders user plugin forms and plugin-hosted links", () => {
    usePluginSettingsListMock.mockReturnValue({
      data: {
        installations: [
          {
            id: 11,
            plugin_id: "example.remote",
            version: "1.2.3",
          },
        ],
      },
      isLoading: false,
    });
    usePluginSettingsDetailMock.mockReturnValue({
      data: {
        installation: {
          id: 11,
          plugin_id: "example.remote",
          version: "1.2.3",
          user_config_schema: [
            { key: "theme", title: "Theme", json_schema: '{"type":"string"}' },
          ],
          routes: [
            {
              id: "panel",
              method: "GET",
              path: "/panel",
              access: "authenticated",
              navigable: true,
              navigation_label: "Open Panel",
              navigation_kind: "user",
              static_asset: false,
            },
          ],
          assets: [
            { path: "assets/admin.js", content_type: "application/javascript" },
          ],
        },
        values: { theme: "ocean" },
      },
      isLoading: false,
    });
    useUpdatePluginSettingsMock.mockReturnValue({
      mutate: vi.fn(),
      isPending: false,
    });

    const markup = renderToStaticMarkup(
      <MemoryRouter>
        <PluginSettings />
      </MemoryRouter>,
    );

    expect(markup).toContain("example.remote");
    expect(markup).toContain("Theme");
    expect(markup).toContain("ocean");
    expect(markup).toContain("Open Panel");
  });
});
