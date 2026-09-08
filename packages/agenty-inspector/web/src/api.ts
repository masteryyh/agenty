import { useEffect, useState } from "react";

export function endpoint(
    path: string,
    params: { [key: string]: string | number | undefined } = {},
): string {
    const url = new URL("/api/v1" + path, window.location.origin);
    for (const [key, value] of Object.entries(params)) {
        if (value !== undefined && value !== "") {
            url.searchParams.set(key, String(value));
        }
    }
    return url.pathname + url.search;
}

export function useResource<T>(url: string | null, refresh = 0) {
    const [state, setState] = useState<{
        key: string;
        data: T | null;
        error: string;
        loading: boolean;
    }>({ key: "", data: null, error: "", loading: false });
    const key = (url ?? "") + ":" + refresh;
    useEffect(() => {
        if (!url) {
            return;
        }
        const controller = new AbortController();
        setState({ key, data: null, error: "", loading: true });
        void fetch(url, { signal: controller.signal })
            .then(async (response) => {
                const data: unknown = await response.json();
                if (!response.ok) {
                    const message =
                        typeof data === "object" &&
                        data !== null &&
                        "message" in data &&
                        typeof data.message === "string"
                            ? data.message
                            : "Request failed (" + response.status + ")";
                    throw new Error(message);
                }
                if (!controller.signal.aborted) {
                    // The wire types are generated from the Go DTOs and checked in CI.
                    setState({
                        key,
                        data: data as T,
                        error: "",
                        loading: false,
                    });
                }
            })
            .catch((error: unknown) => {
                if (!controller.signal.aborted) {
                    setState({
                        key,
                        data: null,
                        error:
                            error instanceof Error
                                ? error.message
                                : String(error),
                        loading: false,
                    });
                }
            });
        return () => controller.abort();
    }, [url, key]);
    return state.key === key
        ? state
        : { data: null, error: "", loading: Boolean(url) };
}

export function useDebounced(value: string, delay = 200): string {
    const [result, setResult] = useState(value);
    useEffect(() => {
        const timer = window.setTimeout(() => setResult(value), delay);
        return () => window.clearTimeout(timer);
    }, [value, delay]);
    return result;
}

export function formatTime(value: string | null | undefined): string {
    if (!value || value.startsWith("0001")) {
        return "—";
    }
    return new Date(value).toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
    });
}

export function bytes(value: number): string {
    if (value < 1024) {
        return value + " B";
    }
    if (value < 1024 * 1024) {
        return (value / 1024).toFixed(1) + " KB";
    }
    return (value / 1024 / 1024).toFixed(1) + " MB";
}
