import { describe, expect, it } from "vitest";
import { resolveCurrentProfile } from "./useCurrentProfile";
import type { Profile } from "@/api/types";

function makeProfile(overrides: Partial<Profile> = {}): Profile {
  return {
    id: "profile-1",
    name: "Admin",
    avatar: "",
    has_pin: false,
    is_child: false,
    is_primary: true,
    max_content_rating: "",
    quality_preference: "4k",
    language: "",
    subtitle_language: "",
    subtitle_mode: "auto",
    auto_skip_intro: false,
    auto_skip_credits: false,
    library_restrictions_enabled: false,
    allowed_library_ids: null,
    max_playback_quality: "",
    created_at: "2026-03-20T03:00:45Z",
    updated_at: "2026-03-20T17:36:33Z",
    ...overrides,
  };
}

describe("resolveCurrentProfile", () => {
  it("prefers the freshly fetched profile over the stale auth copy", () => {
    const authProfile = makeProfile({ quality_preference: "4k" });
    const fetchedProfile = makeProfile({ quality_preference: "720p" });

    expect(resolveCurrentProfile([fetchedProfile], authProfile)?.quality_preference).toBe("720p");
  });

  it("falls back to the auth profile when the query list is unavailable", () => {
    const authProfile = makeProfile({ quality_preference: "4k" });

    expect(resolveCurrentProfile([], authProfile)?.quality_preference).toBe("4k");
  });
});
