/**
 * Browser localStorage accessors for the Prairie web client.
 *
 * STORAGE_KEYS string values are a durable client contract — do not rename them.
 * Renaming a key silently orphans existing user data (auth session, prefs, etc.).
 *
 * Auth / profile session keys listed in UPGRADE_PRESERVED_KEYS must survive
 * client upgrades, deploys, and any storage schema migration. There is no
 * clear-on-deploy / version-bump wipe path for these keys.
 */

const STORAGE_KEYS = {
  ACCESS_TOKEN: "access_token",
  REFRESH_TOKEN: "refresh_token",
  PROFILE_ID: "profile_id",
  PROFILE_TOKEN: "profile_token",
  CURRENT_PROFILE: "current_profile",
  DEVICE_ID: "prairie-device-id",
  /** Admin session stash while impersonating — must survive schema bumps. */
  IMPERSONATION_ADMIN_SESSION: "impersonation_admin_session",
  VOLUME: "player-volume",
  MUTED: "player-muted",
  AUDIOBOOK_SKIP_BACK: "audiobook-skip-back",
  AUDIOBOOK_SKIP_FORWARD: "audiobook-skip-forward",
  AUDIOBOOK_SMART_REWIND: "audiobook-smart-rewind",
  AUDIOBOOK_RATES: "audiobook-rates",
  THEME: "prairie-theme",
  UI_TEXT_SCALE: "prairie-ui-text-scale",
  UI_TEXT_WEIGHT: "prairie-ui-text-weight",
  UI_HIGH_CONTRAST: "prairie-ui-high-contrast",
  UI_CUSTOM_THEME_VARS: "prairie-custom-theme-vars",
  UI_DATE_FORMAT: "prairie-ui-date-format",
  UI_TIME_FORMAT: "prairie-ui-time-format",
  UI_DATETIME_FORMAT_OWNER: "prairie-ui-datetime-format-owner",
  UI_CUSTOM_CSS: "prairie-custom-css",
  CALENDAR_PRESET: "calendar:preset",
} as const;

type StorageKey = (typeof STORAGE_KEYS)[keyof typeof STORAGE_KEYS];

/**
 * Keys that must never be cleared by a client upgrade / storage schema bump.
 * refresh_token + profile identity keep the user signed in across deploys.
 */
export const UPGRADE_PRESERVED_KEYS = [
  STORAGE_KEYS.REFRESH_TOKEN,
  STORAGE_KEYS.PROFILE_ID,
  STORAGE_KEYS.PROFILE_TOKEN,
  STORAGE_KEYS.CURRENT_PROFILE,
  STORAGE_KEYS.DEVICE_ID,
  STORAGE_KEYS.IMPERSONATION_ADMIN_SESSION,
] as const;

export type UpgradePreservedKey = (typeof UPGRADE_PRESERVED_KEYS)[number];

/**
 * Storage schema version for future migrations.
 * Bumping this must not clear UPGRADE_PRESERVED_KEYS — migrations may only
 * rewrite or remove non-session preference keys.
 */
export const STORAGE_SCHEMA_VERSION = 1;
const STORAGE_SCHEMA_VERSION_KEY = "prairie-storage-schema-version";

/**
 * Record the current storage schema version without touching session keys.
 * Intentionally does not clear localStorage on version change.
 */
export function ensureStorageSchema(): number {
  try {
    const raw = localStorage.getItem(STORAGE_SCHEMA_VERSION_KEY);
    const current = raw == null || raw === "" ? 0 : Number(raw);
    const isValidInteger = Number.isFinite(current) && Number.isInteger(current) && current >= 0;
    if (!isValidInteger || current < STORAGE_SCHEMA_VERSION) {
      localStorage.setItem(STORAGE_SCHEMA_VERSION_KEY, String(STORAGE_SCHEMA_VERSION));
    }
    return STORAGE_SCHEMA_VERSION;
  } catch {
    return STORAGE_SCHEMA_VERSION;
  }
}

/** True when a storage key is an auth/session key that survives upgrades. */
export function isUpgradePreservedKey(key: string): key is UpgradePreservedKey {
  return (UPGRADE_PRESERVED_KEYS as readonly string[]).includes(key);
}

function get(key: StorageKey): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function set(key: StorageKey, value: string): void {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Storage full or unavailable
  }
}

function remove(key: StorageKey): void {
  try {
    localStorage.removeItem(key);
  } catch {
    // Storage unavailable
  }
}

export const storage = { KEYS: STORAGE_KEYS, get, set, remove };
