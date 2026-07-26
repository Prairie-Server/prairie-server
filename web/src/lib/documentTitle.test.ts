import { afterEach, describe, expect, it } from "vitest";

import {
  APP_DOCUMENT_TITLE,
  formatDocumentTitle,
  resolveAdminDocumentTitle,
  resolveSettingsDocumentTitle,
  setActiveDocumentTitleLabel,
  setAppDocumentTitle,
} from "./documentTitle";

describe("formatDocumentTitle", () => {
  it("returns the app name when no label is provided", () => {
    expect(formatDocumentTitle()).toBe(APP_DOCUMENT_TITLE);
    expect(formatDocumentTitle("")).toBe(APP_DOCUMENT_TITLE);
    expect(formatDocumentTitle("   ")).toBe(APP_DOCUMENT_TITLE);
  });

  it("prefixes the current page label", () => {
    expect(formatDocumentTitle("Inception")).toBe("Inception · Prairie");
  });
});

describe("setAppDocumentTitle / setActiveDocumentTitleLabel", () => {
  afterEach(() => {
    setActiveDocumentTitleLabel(null);
    setAppDocumentTitle("Prairie");
  });

  it("updates the app name and recomputes document.title from the active label", () => {
    setActiveDocumentTitleLabel("Library");
    setAppDocumentTitle("My Silo");
    expect(document.title).toBe("Library · My Silo");
    expect(formatDocumentTitle("Library")).toBe("Library · My Silo");
  });

  it("falls back to Prairie when given an empty branding name", () => {
    setActiveDocumentTitleLabel(undefined);
    setAppDocumentTitle("");
    expect(document.title).toBe("Prairie");
  });
});

describe("resolveSettingsDocumentTitle", () => {
  it("resolves nested settings route labels", () => {
    expect(resolveSettingsDocumentTitle("/settings/playback")).toBe("Playback Settings");
    expect(resolveSettingsDocumentTitle("/settings/home-screen")).toBe("Home Screen Settings");
    expect(resolveSettingsDocumentTitle("/settings/appearance")).toBe("Appearance Settings");
    expect(resolveSettingsDocumentTitle("/settings/webhook-sync")).toBe("Webhook Sync Settings");
  });

  it("falls back to the base settings title", () => {
    expect(resolveSettingsDocumentTitle("/settings")).toBe("Settings");
    expect(resolveSettingsDocumentTitle("/settings/unknown")).toBe("Settings");
  });
});

describe("resolveAdminDocumentTitle", () => {
  it("resolves major admin sections", () => {
    expect(resolveAdminDocumentTitle("/admin")).toBe("Admin");
    expect(resolveAdminDocumentTitle("/admin/collections")).toBe("Admin Collections");
    expect(resolveAdminDocumentTitle("/admin/diagnostics")).toBe("Admin Client Diagnostics");
    expect(resolveAdminDocumentTitle("/admin/tasks/refresh-metadata")).toBe("Admin Task");
    expect(resolveAdminDocumentTitle("/admin/users/42")).toBe("Admin User");
    expect(resolveAdminDocumentTitle("/admin/unknown")).toBe("Admin");
  });

  it("handles editor routes with clearer labels", () => {
    expect(resolveAdminDocumentTitle("/admin/collections/new")).toBe("New Admin Collection");
    expect(resolveAdminDocumentTitle("/admin/collections/col-7/edit")).toBe(
      "Edit Admin Collection",
    );
  });
});
