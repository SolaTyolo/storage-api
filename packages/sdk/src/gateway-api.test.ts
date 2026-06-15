import { describe, expect, it, vi } from "vitest";
import { GatewayApi, GatewayApiError } from "./gateway-api.js";

describe("GatewayApi", () => {
  it("posts JSON with Authorization and parses response", async () => {
    const fetchImpl = vi.fn(async () => {
      return new Response(JSON.stringify({ uploadUrl: "https://s3/put", cdnUrl: "https://cdn/a" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    const api = new GatewayApi("http://localhost:8080/api/v1", "tok", fetchImpl as unknown as typeof fetch);
    const res = await api.presignPut({
      bucket: "uploads",
      key: "a.png",
      contentType: "image/png",
    });
    expect(res.uploadUrl).toContain("https://s3/put");
    expect(fetchImpl).toHaveBeenCalledOnce();
    const [url, init] = fetchImpl.mock.calls[0]!;
    expect(url).toBe("http://localhost:8080/api/v1/presign/put");
    expect((init as RequestInit).headers).toMatchObject({
      Authorization: "Bearer tok",
    });
  });

  it("throws GatewayApiError on non-2xx", async () => {
    const fetchImpl = vi.fn(async () => {
      return new Response(JSON.stringify({ error: "forbidden", message: "key prefix not allowed" }), {
        status: 403,
        headers: { "Content-Type": "application/json" },
      });
    });
    const api = new GatewayApi("http://gw/api/v1", "tok", fetchImpl as unknown as typeof fetch);
    await expect(api.objectHead({ bucket: "uploads", key: "x" })).rejects.toBeInstanceOf(
      GatewayApiError,
    );
  });
});
