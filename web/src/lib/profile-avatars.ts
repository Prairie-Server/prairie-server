import type { Profile } from "@/api/types";

export interface ProfileAvatarPreset {
  id: string;
  label: string;
  previewUrl: string;
  styleId: string;
}

export interface ProfileAvatarStyleOption {
  id: string;
  label: string;
  summary: string;
}

const LEGACY_PROFILE_AVATAR_PRESETS: ProfileAvatarPreset[] = [
  {
    id: "avatar-1",
    label: "Sky Fox",
    previewUrl: "/profile-avatars/avatar-1.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-2",
    label: "Comet Cat",
    previewUrl: "/profile-avatars/avatar-2.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-3",
    label: "Star Bear",
    previewUrl: "/profile-avatars/avatar-3.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-4",
    label: "Neon Pup",
    previewUrl: "/profile-avatars/avatar-4.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-5",
    label: "Orbit Bunny",
    previewUrl: "/profile-avatars/avatar-5.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-6",
    label: "Solar Owl",
    previewUrl: "/profile-avatars/avatar-6.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-7",
    label: "Nova Gecko",
    previewUrl: "/profile-avatars/avatar-7.svg",
    styleId: "legacy",
  },
  {
    id: "avatar-8",
    label: "Pixel Penguin",
    previewUrl: "/profile-avatars/avatar-8.svg",
    styleId: "legacy",
  },
];

export const PROFILE_AVATAR_STYLES: ProfileAvatarStyleOption[] = [
  {
    id: "identicon",
    label: "Identicon",
    summary: "Geometric, technical, high-contrast patterns",
  },
  {
    id: "initials",
    label: "Initials",
    summary: "Clean letter-based avatars with bold backgrounds",
  },
  {
    id: "bottts-neutral",
    label: "Bottts Neutral",
    summary: "Cute modular robot-style icons",
  },
  {
    id: "fun-emoji",
    label: "Fun Emoji",
    summary: "Big, colorful, instantly readable faces",
  },
  {
    id: "pixel-art-neutral",
    label: "Pixel Art Neutral",
    summary: "Retro pixel faces with lots of variation",
  },
];

const PROFILE_AVATAR_SEED_ADJECTIVES = [
  "cosmic",
  "jelly",
  "starlight",
  "mango",
  "bubble",
  "neon",
  "marble",
  "comet",
  "snappy",
  "pepper",
  "glimmer",
  "ripple",
  "pocket",
  "candy",
  "ember",
  "mochi",
  "sprout",
  "twinkle",
  "whiz",
  "plasma",
  "mint",
  "velvet",
  "crunchy",
  "sunbeam",
  "toffee",
  "splash",
  "boomer",
  "lunar",
  "fizzy",
  "biscuit",
  "rocket",
  "cobalt",
];

const PROFILE_AVATAR_SEED_NOUNS = [
  "otter",
  "rocket",
  "fox",
  "cookie",
  "gecko",
  "bunny",
  "penguin",
  "puffin",
  "tiger",
  "panda",
  "saturn",
  "wizard",
  "skater",
  "nebula",
  "robot",
  "pirate",
  "meteor",
  "sprite",
  "pebble",
  "nova",
  "dragon",
  "donut",
  "parrot",
  "panther",
  "goblin",
  "muffin",
  "laser",
  "pickle",
  "falcon",
  "sunset",
  "toaster",
  "bandit",
];

const PROFILE_AVATAR_OPTION_COUNT = 18;

function titleCase(value: string) {
  return value.replace(/\b\w/g, (char) => char.toUpperCase());
}

export function avatarPresetRef(id: string) {
  return `preset:${id}`;
}

export function diceBearPresetId(styleId: string, seed: string) {
  return `dicebear:${styleId}:${seed}`;
}

export function buildDiceBearAvatarUrl(
  styleId: string,
  seed: string,
  size = 128,
) {
  const query = new URLSearchParams({
    seed,
    size: String(size),
    radius: "24",
    backgroundType: "gradientLinear",
  });
  return `https://api.dicebear.com/9.x/${styleId}/svg?${query.toString()}`;
}

export function parseDiceBearPresetId(presetId: string | null | undefined) {
  if (!presetId || !presetId.startsWith("dicebear:")) {
    return null;
  }
  const parts = presetId.split(":");
  if (parts.length !== 3) {
    return null;
  }
  const styleId = parts[1];
  const seed = parts[2];
  if (!styleId || !seed) {
    return null;
  }
  if (
    !PROFILE_AVATAR_STYLES.some((style) => style.id === styleId) ||
    seed.trim() === ""
  ) {
    return null;
  }
  return { styleId, seed };
}

function createDiceBearPreset(
  styleId: string,
  seed: string,
): ProfileAvatarPreset {
  return {
    id: diceBearPresetId(styleId, seed),
    label: titleCase(seed.replace(/-/g, " ")),
    previewUrl: buildDiceBearAvatarUrl(styleId, seed),
    styleId,
  };
}

export function buildProfileAvatarPresetBatch(
  styleId: string,
  batch = 0,
): ProfileAvatarPreset[] {
  const normalizedStyle = PROFILE_AVATAR_STYLES.some(
    (style) => style.id === styleId,
  )
    ? styleId
    : (PROFILE_AVATAR_STYLES[0]?.id ?? "identicon");

  return Array.from({ length: PROFILE_AVATAR_OPTION_COUNT }, (_, index) => {
    const adjective =
      PROFILE_AVATAR_SEED_ADJECTIVES[
        (batch * 7 + index * 3 + normalizedStyle.length) %
          PROFILE_AVATAR_SEED_ADJECTIVES.length
      ];
    const noun =
      PROFILE_AVATAR_SEED_NOUNS[
        (batch * 11 + index * 5 + normalizedStyle.length * 2) %
          PROFILE_AVATAR_SEED_NOUNS.length
      ];
    return createDiceBearPreset(normalizedStyle, `${adjective}-${noun}`);
  });
}

export function resolveProfileAvatarPreset(
  avatarRef: string | null | undefined,
): ProfileAvatarPreset | null {
  if (!avatarRef) {
    return null;
  }
  const diceBear = parseDiceBearPresetId(avatarRef);
  if (diceBear) {
    return createDiceBearPreset(diceBear.styleId, diceBear.seed);
  }
  return (
    LEGACY_PROFILE_AVATAR_PRESETS.find((preset) => preset.id === avatarRef) ??
    null
  );
}

export function parseProfileAvatarPresetRef(
  avatarRef: string | null | undefined,
) {
  if (!avatarRef) {
    return "";
  }
  const normalized = avatarRef.startsWith("preset:")
    ? avatarRef.slice("preset:".length)
    : avatarRef;
  return resolveProfileAvatarPreset(normalized)?.id ?? "";
}

export function resolveProfileAvatarImage(
  profile: Pick<Profile, "avatar_url"> | null | undefined,
) {
  return profile?.avatar_url ?? "";
}
