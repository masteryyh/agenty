// Single source of truth for the agenty TUI palette: a warm neutral surface
// ramp plus one amber accent, with chromatic color reserved for semantic
// status. Dark only. Every token is an absolute #RRGGBB value, so the UI reads
// the same regardless of the background the user's terminal is configured with.
//
// Do not declare color literals anywhere else; theme.test.ts fails if a hex or
// terminal color name appears outside this file.

const palette = {
    // Surfaces: warm near-black, evenly stepped.
    base: "#16130F",
    surface: "#1D1913",
    surfaceHover: "#25201A",
    surfaceRaised: "#2D2721",
    border: "#3A322A",
    borderStrong: "#625A50",
    // Text.
    text: "#E8E2D9",
    textMuted: "#A79C8C",
    textFaint: "#857A6A",
    // Accent ramp: one hue, two steps.
    accent: "#D98E4A",
    accentBright: "#E8A85F",
    // Status.
    success: "#8FB05A",
    warning: "#DCB842",
    danger: "#D96A5F",
    info: "#B7A99B",
} as const;

/**
 * Semantic tokens. Reference `theme.x` from components; never import
 * `palette` or declare raw color values outside this module.
 */
export const theme = {
    // Fills.
    base: palette.base,
    surface: palette.surface,
    surfaceHover: palette.surfaceHover,
    surfaceRaised: palette.surfaceRaised,
    // Lines.
    border: palette.border,
    borderStrong: palette.borderStrong,
    // Foregrounds.
    text: palette.text,
    textMuted: palette.textMuted,
    textFaint: palette.textFaint,
    // Brand + focus.
    accent: palette.accent,
    accentBright: palette.accentBright,
    // Foreground for the row/control that currently owns focus.
    selection: palette.accentBright,
    // Status.
    success: palette.success,
    warning: palette.warning,
    danger: palette.danger,
    info: palette.info,
} as const;

export type ThemeToken = keyof typeof theme;

export type ToolStatus = "pending" | "success" | "error" | "cancelled";

export type ShellStream = "stdout" | "stderr" | "empty" | "pending" | "newline";

export function statusColor(status: ToolStatus): string {
    if (status === "cancelled") {
        return theme.textMuted;
    }
    if (status === "success") {
        return theme.success;
    }
    if (status === "error") {
        return theme.danger;
    }
    return theme.accent;
}

export function outputColor(stream: ShellStream): string | undefined {
    return stream === "stderr"
        ? theme.danger
        : stream === "empty" || stream === "pending" || stream === "newline"
            ? theme.textMuted
            : undefined;
}

/** Ordinal ramp: faint -> muted -> text -> accent -> accentBright. */
export function effortColor(level: string): string {
    switch (level) {
        case "low":
            return theme.textFaint;
        case "medium":
            return theme.textMuted;
        case "high":
            return theme.text;
        case "xhigh":
            return theme.accent;
        case "max":
            return theme.accentBright;
        default:
            return theme.textMuted;
    }
}

/** Disabled wins, then the danger tone, then the focused action. */
export function actionColor(state: {
    disabled?: boolean;
    tone?: "default" | "danger";
    active?: boolean;
}): string {
    if (state.disabled) {
        return theme.textFaint;
    }
    if (state.tone === "danger") {
        return theme.danger;
    }
    return state.active ? theme.selection : theme.textMuted;
}
