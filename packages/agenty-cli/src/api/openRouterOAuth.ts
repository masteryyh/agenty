import { createHash, randomBytes } from "node:crypto";

const authenticationCompletePage = `<!doctype html>
<html lang="en">
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Authentication complete · Agenty</title>
    <link rel="icon" href="data:,">
    <style>
        :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
        body { margin: 0; min-height: 100vh; display: grid; place-items: center; }
        main { max-width: 32rem; padding: 2rem; text-align: center; }
        h1 { font-size: 1.75rem; font-weight: 600; }
        p { line-height: 1.6; }
        .brand { font-size: .875rem; letter-spacing: .15em; opacity: .6; }
    </style>
</head>
<body>
    <main>
        <p class="brand">AGENTY</p>
        <h1>Authentication complete</h1>
        <p>You can close this page and return to agenty.</p>
    </main>
</body>
</html>`;

export async function loginWithOpenRouter(
    signal: AbortSignal,
): Promise<string> {
    const verifier = randomBytes(32).toString("base64url");
    const challenge = createHash("sha256").update(verifier).digest("base64url");
    const state = randomBytes(24).toString("base64url");
    const abortSignal = AbortSignal.any([signal, AbortSignal.timeout(5 * 60 * 1000)]);

    let resolveKey: (key: string) => void = () => undefined;
    let rejectKey: (cause: Error) => void = () => undefined;
    let callbackHandled = false;
    const keyPromise = new Promise<string>((resolve, reject) => {
        resolveKey = resolve;
        rejectKey = reject;
    });
    const server = Bun.serve({
        hostname: "localhost",
        port: 0,
        fetch: async (request) => {
            const callback = new URL(request.url);
            if (callback.pathname !== `/oauth/callback/${state}`) {
                return new Response("Not found", { status: 404 });
            }
            if (abortSignal.aborted) {
                return new Response("Authorization was cancelled", { status: 410 });
            }
            if (callbackHandled) {
                return new Response("Authorization has already been completed", { status: 409 });
            }
            const error = callback.searchParams.get("error");
            if (error) {
                callbackHandled = true;
                rejectKey(new Error("OpenRouter authorization was cancelled."));
                return new Response("Authorization was cancelled. You can return to agenty.");
            }
            const code = callback.searchParams.get("code");
            if (!code) {
                return new Response("Authorization code is missing", { status: 400 });
            }

            callbackHandled = true;
            try {
                const response = await fetch("https://openrouter.ai/api/v1/auth/keys", {
                    method: "POST",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify({
                        code,
                        code_verifier: verifier,
                        code_challenge_method: "S256",
                    }),
                    signal: abortSignal,
                });
                if (!response.ok) {
                    throw new Error(`OpenRouter key exchange failed (HTTP ${response.status}).`);
                }
                const result: unknown = await response.json();
                if (!result || typeof result !== "object" || !("key" in result) || typeof result.key !== "string" || result.key === "") {
                    throw new Error("OpenRouter returned no API key.");
                }
                resolveKey(result.key);
                return new Response(authenticationCompletePage, {
                    headers: {
                        "Content-Type": "text/html; charset=utf-8",
                        "Cache-Control": "no-store",
                    },
                });
            } catch (cause) {
                rejectKey(cause instanceof Error ? cause : new Error(String(cause)));
                return new Response("Authorization failed. You can return to agenty.", { status: 502 });
            }
        },
    });

    const callbackURL = `http://localhost:${server.port}/oauth/callback/${state}`;
    const authorizationURL = buildOpenRouterAuthorizationURL(callbackURL, challenge);

    const handleAbort = () => rejectKey(new Error("OpenRouter authorization timed out or was cancelled."));
    abortSignal.addEventListener("abort", handleAbort, { once: true });
    try {
        openBrowser(authorizationURL.toString());
        return await keyPromise;
    } finally {
        abortSignal.removeEventListener("abort", handleAbort);
        // Let the browser receive the callback response before closing its connection.
        await server.stop();
    }
}

export function buildOpenRouterAuthorizationURL(callbackURL: string, challenge: string): URL {
    const authorizationURL = new URL("https://openrouter.ai/auth");
    authorizationURL.searchParams.set("callback_url", callbackURL);
    authorizationURL.searchParams.set("code_challenge", challenge);
    authorizationURL.searchParams.set("code_challenge_method", "S256");
    authorizationURL.searchParams.set("key_label", "Agenty");
    return authorizationURL;
}

function openBrowser(url: string): void {
    const command = process.platform === "darwin" ? "open" : process.platform === "win32" ? "cmd" : "xdg-open";
    const args = process.platform === "win32" ? ["/c", "start", "", url] : [url];
    void Bun.spawn([command, ...args], { stdout: "ignore", stderr: "ignore" });
}
