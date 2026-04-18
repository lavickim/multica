export type ShiftosBootstrapSession = {
  token: string;
  workspaceId: string;
};

function decodeBase64Url(value: string): string {
  if (typeof window !== "undefined" && typeof window.atob === "function") {
    const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
    const padded = normalized.padEnd(Math.ceil(normalized.length / 4) * 4, "=");
    return decodeURIComponent(
      Array.from(window.atob(padded))
        .map((char) => `%${char.charCodeAt(0).toString(16).padStart(2, "0")}`)
        .join(""),
    );
  }

  return Buffer.from(value, "base64url").toString("utf-8");
}

export function readShiftosBootstrapSession(hash: string): ShiftosBootstrapSession | null {
  const rawHash = hash.startsWith("#") ? hash.slice(1) : hash;
  if (!rawHash) {
    return null;
  }

  const params = new URLSearchParams(rawHash);
  const encoded = params.get("shiftos_bootstrap")?.trim() || "";
  if (!encoded) {
    return null;
  }

  try {
    const payload = JSON.parse(decodeBase64Url(encoded)) as Record<string, unknown>;
    const token = typeof payload.token === "string" ? payload.token.trim() : "";
    const workspaceId = typeof payload.workspaceId === "string" ? payload.workspaceId.trim() : "";
    if (!token || !workspaceId) {
      return null;
    }
    return { token, workspaceId };
  } catch {
    return null;
  }
}

export function applyShiftosBootstrapSession(win: Window): ShiftosBootstrapSession | null {
  const session = readShiftosBootstrapSession(win.location.hash);
  if (!session) {
    return null;
  }

  win.localStorage.setItem("multica_token", session.token);
  win.localStorage.setItem("multica_workspace_id", session.workspaceId);
  win.history.replaceState({}, "", `${win.location.pathname}${win.location.search}`);
  return session;
}
