import { storage } from "../utils/storage";

export interface StoredImpersonationAdminSession {
  accessToken: string;
  refreshToken: string;
  returnPath: string;
}

export function saveStoredImpersonationAdminSession(
  session: StoredImpersonationAdminSession,
): void {
  storage.set(
    storage.KEYS.IMPERSONATION_ADMIN_SESSION,
    JSON.stringify(session),
  );
}

export function loadStoredImpersonationAdminSession(): StoredImpersonationAdminSession | null {
  try {
    const rawSession = storage.get(storage.KEYS.IMPERSONATION_ADMIN_SESSION);
    if (!rawSession) {
      return null;
    }

    const parsed = JSON.parse(
      rawSession,
    ) as Partial<StoredImpersonationAdminSession>;
    if (
      typeof parsed.accessToken !== "string" ||
      typeof parsed.refreshToken !== "string" ||
      typeof parsed.returnPath !== "string"
    ) {
      return null;
    }

    return {
      accessToken: parsed.accessToken,
      refreshToken: parsed.refreshToken,
      returnPath: parsed.returnPath,
    };
  } catch {
    return null;
  }
}

export function clearStoredImpersonationAdminSession(): void {
  storage.remove(storage.KEYS.IMPERSONATION_ADMIN_SESSION);
}
