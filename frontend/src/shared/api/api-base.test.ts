import { afterEach, expect, it, vi } from "vitest";

afterEach(() => { vi.unstubAllEnvs(); vi.unstubAllGlobals(); vi.resetModules(); });

it("uses the configured API origin for streaming with the real session JWT", async () => {
  vi.stubEnv("VITE_API_BASE_URL", "https://api.example.test/");
  vi.resetModules();
  const { apiResponse, setAccessToken, setProjectContext } = await import("./client");
  const fetchMock = vi.fn().mockResolvedValue(new Response("data: {}\n\n"));
  vi.stubGlobal("fetch", fetchMock);
  setAccessToken("origin-jwt");
  setProjectContext({ project: "alpha", environment: "production" });
  await apiResponse("/api/v1/live/stream", "session");
  expect(fetchMock.mock.calls[0][0]).toBe("https://api.example.test/api/v1/live/stream");
  expect(new Headers(fetchMock.mock.calls[0][1].headers).get("Authorization")).toBe("Bearer origin-jwt");
});
