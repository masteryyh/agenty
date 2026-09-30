import { createHash } from "node:crypto";
import { lookup } from "node:dns/promises";

import { describe, expect, spyOn, test } from "bun:test";

import { buildOpenRouterAuthorizationURL, loginWithOpenRouter } from "./openRouterOAuth";

async function receiveCallback(exchangeResponse: Response) {
    const request = globalThis.fetch;
    const browserProcess = Bun.spawn([process.execPath, "--version"], { stdout: "ignore", stderr: "ignore" });
    const browser = spyOn(Bun, "spawn").mockReturnValue(browserProcess);
    const exchange = spyOn(globalThis, "fetch").mockResolvedValue(exchangeResponse);
    const controller = new AbortController();
    const login = loginWithOpenRouter(controller.signal);
    // Observe a rejected login immediately while the callback response is read.
    const result = Promise.allSettled([login]);

    try {
        const command = browser.mock.calls[0]?.[0];
        if (!Array.isArray(command)) {
            throw new Error("OAuth did not open an authorization URL.");
        }
        const authorization = new URL(command.at(-1) ?? "");
        const callback = new URL(authorization.searchParams.get("callback_url") ?? "");
        callback.searchParams.set("code", "fixture-code");
        // Match the address used by the localhost listener instead of fetch's IPv4 preference.
        const address = await lookup(callback.hostname);
        callback.hostname = address.family === 6 ? `[${address.address}]` : address.address;

        const response = await request(callback, { signal: AbortSignal.timeout(2_000) });
        const body = await response.text();
        const [key] = await result;
        expect(exchange).toHaveBeenCalledTimes(1);
        return { key, response, body };
    } finally {
        controller.abort();
        await result;
        browser.mockRestore();
        exchange.mockRestore();
        await browserProcess.exited;
    }
}

describe("OpenRouter OAuth authorization URL", () => {
    test("includes the PKCE challenge and callback path", () => {
        const verifier = "test-verifier";
        const challenge = createHash("sha256").update(verifier).digest("base64url");
        const url = buildOpenRouterAuthorizationURL(
            "http://localhost:43127/oauth/callback/test-state",
            challenge,
        );

        expect(url.origin + url.pathname).toBe("https://openrouter.ai/auth");
        expect(url.searchParams.get("callback_url")).toBe("http://localhost:43127/oauth/callback/test-state");
        expect(url.searchParams.get("code_challenge")).toBe(challenge);
        expect(url.searchParams.get("code_challenge_method")).toBe("S256");
    });
});

describe("OpenRouter OAuth callback response", () => {
    test("delivers the complete success page before closing the callback server", async () => {
        const { key, response, body } = await receiveCallback(Response.json({ key: "fixture-api-key" }));

        expect(key).toEqual({ status: "fulfilled", value: "fixture-api-key" });
        expect(response.status).toBe(200);
        expect(response.headers.get("content-type")).toBe("text/html; charset=utf-8");
        expect(body).toContain("Authentication complete");
        expect(body).toContain("You can close this page and return to agenty.");
        expect(body).toContain("</html>");
        expect(body).not.toContain("fixture-api-key");
        expect(body).not.toContain("fixture-code");
    });

    test("delivers the failure response when the key exchange is rejected", async () => {
        const { key, response, body } = await receiveCallback(new Response("Rejected", { status: 403 }));

        expect(key.status).toBe("rejected");
        expect(response.status).toBe(502);
        expect(body).toBe("Authorization failed. You can return to agenty.");
    });
});
