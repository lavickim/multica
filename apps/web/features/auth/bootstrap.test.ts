import { beforeEach, describe, expect, it } from "vitest";

import { applyShiftosBootstrapSession, readShiftosBootstrapSession } from "./bootstrap";

function encodePayload(value: object): string {
  return Buffer.from(JSON.stringify(value), "utf-8").toString("base64url");
}

describe("shiftos bootstrap session", () => {
  beforeEach(() => {
    window.localStorage.clear();
    window.history.replaceState({}, "", "/issues");
  });

  it("reads a valid bootstrap session from the location hash", () => {
    const session = readShiftosBootstrapSession(
      `#shiftos_bootstrap=${encodePayload({ token: "mul_token", workspaceId: "ws-1" })}`,
    );

    expect(session).toEqual({
      token: "mul_token",
      workspaceId: "ws-1",
    });
  });

  it("hydrates localStorage and clears the hash when a bootstrap session is present", () => {
    window.history.replaceState(
      {},
      "",
      `/issues?view=board#shiftos_bootstrap=${encodePayload({ token: "mul_token", workspaceId: "ws-1" })}`,
    );

    const session = applyShiftosBootstrapSession(window);

    expect(session).toEqual({
      token: "mul_token",
      workspaceId: "ws-1",
    });
    expect(window.localStorage.getItem("multica_token")).toBe("mul_token");
    expect(window.localStorage.getItem("multica_workspace_id")).toBe("ws-1");
    expect(window.location.hash).toBe("");
    expect(window.location.pathname + window.location.search).toBe("/issues?view=board");
  });
});
