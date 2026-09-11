import { useCallback, useEffect, useRef, useState } from "react";

import type { AgentyClient } from "../api/client";
import type { SkillDto } from "../api/types";
import type { Command } from "../commands/registry";
import {
    findCommand,
    matchingCommands,
    parseCommandTokens,
    quoteArg,
} from "../commands/registry";

const MAX_ITEMS = 8;

export type Palette =
    | { mode: "none" }
    | {
        mode: "commands";
        matches: Command[];
        highlight: number;
        matchPrefix: string;
    }
    | {
        mode: "args";
        command: Command;
        candidates: string[] | null;
        loading: boolean;
        highlight: number;
    }
    | {
        mode: "skills";
        matches: SkillDto[];
        highlight: number;
        matchStart: number;
        matchEnd: number;
        matchPrefix: string;
    };

export type PaletteAction =
    | { type: "text"; value: string }
    | { type: "skill"; skill: SkillDto; start: number; end: number };

export interface PaletteResult {
    palette: Palette;
    height: number;
    tab: () => PaletteAction | null;
}

export function useCommandPalette(
    value: string,
    cursorOffset: number,
    client: AgentyClient | null,
    skills: SkillDto[],
    skillRanges: Array<{ start: number; end: number }> = [],
): PaletteResult {
    const [cache, setCache] = useState<Record<string, string[]>>({});
    const [loading, setLoading] = useState(false);
    const fetchedRef = useRef<Set<string>>(new Set());
    const [cmdHighlight, setCmdHighlight] = useState(-1);
    const selectedCmdRef = useRef(-1);
    const selectedSkillRef = useRef(-1);
    const pendingTabRef = useRef(false);
    const originalQueryRef = useRef(value);

    const tokens = parseCommandTokens(value);
    const startsSlash = value.startsWith("/");
    const cmdToken = startsSlash ? (tokens[0] ?? value) : "";
    const argPart = tokens.length > 1 ? tokens.slice(1).join(" ") : "";
    const hasSpace = value.includes(" ");
    const exactCmd = cmdToken ? findCommand(cmdToken) : undefined;

    useEffect(() => {
        if (pendingTabRef.current) {
            pendingTabRef.current = false;
            return;
        }
        selectedCmdRef.current = -1;
        selectedSkillRef.current = -1;
        setCmdHighlight(-1);
        originalQueryRef.current = value;
    }, [value]);

    let palette: Palette = { mode: "none" };

    const safeCursor = Math.max(0, Math.min(cursorOffset, value.length));
    // A selected skill is rendered as `$name`, but the whole node is an
    // atomic editor element.  At its trailing boundary the `$` is part of
    // the existing node rather than the beginning of a new completion.
    const cursorAtSkillBoundary = skillRanges.some((range) => range.end === safeCursor);
    const dollarStart = value.lastIndexOf("$", safeCursor - 1);
    const dollarPrefix = dollarStart >= 0 ? value.slice(dollarStart + 1, safeCursor) : "";
    const dollarBoundary = !cursorAtSkillBoundary && dollarStart >= 0 &&
        (dollarStart === 0 || /\s/.test(value[dollarStart - 1] ?? "")) &&
        !dollarPrefix.includes(" ") &&
        !value.slice(dollarStart, safeCursor).includes("\n");
    const skillMatches = dollarBoundary
        ? skills.filter((skill) =>
            skill.name.toLowerCase().includes(dollarPrefix.toLowerCase()) ||
            skill.description.toLowerCase().includes(dollarPrefix.toLowerCase()),
        )
        : [];

    if (skillMatches.length > 0) {
        const lower = dollarPrefix.toLowerCase();
        const exact = skillMatches.findIndex((skill) => skill.name.toLowerCase() === lower);
        const prefix = skillMatches.findIndex((skill) => skill.name.toLowerCase().startsWith(lower));
        const defaultHighlight = exact >= 0 ? exact : prefix >= 0 ? prefix : 0;
        const selectedHighlight = selectedSkillRef.current >= 0 && selectedSkillRef.current < skillMatches.length
            ? selectedSkillRef.current
            : defaultHighlight;
        palette = {
            mode: "skills",
            matches: skillMatches,
            highlight: selectedHighlight,
            matchStart: dollarStart,
            matchEnd: safeCursor,
            matchPrefix: dollarPrefix,
        };
    } else if (startsSlash && exactCmd && exactCmd.completeArgs && hasSpace) {
        const cands = cache[exactCmd.name] ?? null;
        let highlight = 0;
        if (cands) {
            const exact = cands.findIndex((c) => c === argPart);
            if (exact >= 0) {
                highlight = exact;
            } else {
                const lower = argPart.toLowerCase();
                const prefix = cands.findIndex((c) =>
                    c.toLowerCase().startsWith(lower),
                );
                highlight = prefix >= 0 ? prefix : 0;
            }
        }
        palette = {
            mode: "args",
            command: exactCmd,
            candidates: cands,
            loading,
            highlight,
        };
    } else if (startsSlash && !hasSpace) {
        const matchPrefix = pendingTabRef.current
            ? originalQueryRef.current
            : value;
        const matches = matchingCommands(matchPrefix);
        if (matches.length > 0) {
            palette = {
                mode: "commands",
                matches,
                highlight: cmdHighlight,
                matchPrefix,
            };
        }
    }

    const fetchKey = palette.mode === "args" ? palette.command.name : null;
    useEffect(() => {
        if (!fetchKey || !client || fetchedRef.current.has(fetchKey)) {
            return;
        }
        const cmd = findCommand(fetchKey);
        if (!cmd?.completeArgs) {
            return;
        }
        fetchedRef.current.add(fetchKey);
        setLoading(true);
        cmd.completeArgs(client)
            .then((c) => {
                setCache((prev) => ({ ...prev, [fetchKey]: c }));
            })
            .catch(() => {
                fetchedRef.current.delete(fetchKey);
            })
            .finally(() => {
                setLoading(false);
            });
    }, [fetchKey, client]);

    const tab = useCallback((): PaletteAction | null => {
        if (palette.mode === "skills") {
            const next = selectedSkillRef.current < 0
                ? palette.highlight
                : (selectedSkillRef.current + 1) % palette.matches.length;
            selectedSkillRef.current = next;
            const skill = palette.matches[next];
            return skill
                ? { type: "skill", skill, start: palette.matchStart, end: palette.matchEnd }
                : null;
        }
        if (palette.mode === "commands") {
            const names = palette.matches.map((m) => m.name);
            if (names.length === 0) {
                return null;
            }
            const next =
                selectedCmdRef.current < 0
                    ? 0
                    : (selectedCmdRef.current + 1) % names.length;
            selectedCmdRef.current = next;
            setCmdHighlight(next);
            pendingTabRef.current = true;
            return { type: "text", value: names[next] };
        }
        if (palette.mode === "args" && palette.candidates) {
            const cands = palette.candidates;
            if (cands.length === 0) {
                return null;
            }
            const selected = cands.findIndex((c) => c === argPart);
            let next: number;
            if (selected >= 0) {
                next = (selected + 1) % cands.length;
            } else {
                const lower = argPart.toLowerCase();
                const pi = cands.findIndex((c) => c.toLowerCase().startsWith(lower));
                next = pi >= 0 ? pi : 0;
            }
            pendingTabRef.current = true;
            return { type: "text", value: `${cmdToken} ${quoteArg(cands[next])}` };
        }
        return null;
    }, [palette, cmdToken, argPart]);

    let height = 0;
    if (palette.mode === "commands") {
        height = Math.min(palette.matches.length, MAX_ITEMS) + 1;
    } else if (palette.mode === "args") {
        const n = palette.candidates ? Math.min(palette.candidates.length, MAX_ITEMS) : 0;
        height = 1 + (palette.loading && !palette.candidates ? 1 : n);
    } else if (palette.mode === "skills") {
        height = Math.min(palette.matches.length, MAX_ITEMS) + 1;
    }

    return { palette, height, tab };
}
