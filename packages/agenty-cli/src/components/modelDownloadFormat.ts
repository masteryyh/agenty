import type { HubModel } from "../api/types";

export function formatModelBytes(bytes: number): string {
    if (bytes < 1024) {
        return `${bytes} B`;
    }
    const units = ["KiB", "MiB", "GiB", "TiB"];
    let amount = bytes / 1024;
    let unit = 0;
    while (amount >= 1024 && unit < units.length - 1) {
        amount /= 1024;
        unit++;
    }
    return `${amount.toFixed(1)} ${units[unit]}`;
}

export function modelParameterLabel(model: Pick<HubModel, "parameters">): string {
    if (!model.parameters || !Number.isFinite(model.parameters) || model.parameters < 0) {
        return "UNKNOWN";
    }
    return model.parameters >= 1e9 ? `${(model.parameters / 1e9).toFixed(2)}B` : `${(model.parameters / 1e6).toFixed(0)}M`;
}

export function modelFileSizeLabel(bytes?: number): string {
    return bytes !== undefined && Number.isFinite(bytes) && bytes > 0 ? formatModelBytes(bytes) : "UNKNOWN";
}

export function modelDownloadProgress(downloaded: number, total: number, width = 12): string {
    const fraction = total > 0 ? Math.min(Math.max(downloaded / total, 0), 1) : 0;
    const filled = Math.floor(fraction * width);
    return `${"⣿".repeat(filled)}${"⣀".repeat(width - filled)} ${Math.floor(fraction * 100)}%`;
}

