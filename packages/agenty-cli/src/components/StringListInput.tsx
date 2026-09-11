import type { InputRenderable, KeyEvent } from "@opentui/core";
import { useEffect, useRef, useState } from "react";

import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { textWidth, truncateText } from "./Table";
import { Box } from "./ui";
import { TextInput } from "./ui/TextInput";

const TAG_GAP = 1;
const TAG_INPUT_MIN_WIDTH = 8;

export interface StringListInputProps {
    value: string[];
    width: number;
    active?: boolean;
    onChange: (value: string[]) => void;
    onActivate?: () => void;
    onMoveOutside?: (direction: -1 | 1) => void;
    onClose?: () => void;
}

interface TagLayout {
    start: number;
    end: number;
    inputWidth: number;
    leadingWidth: number;
    chipWidths: number[];
}

function chipWidth(value: string, availableWidth: number): number {
    return Math.max(
        Math.min(textWidth(`[${value}]`), Math.max(availableWidth - TAG_INPUT_MIN_WIDTH, 1)),
        1,
    );
}

function layoutTags(value: string[], width: number, selectedTag: number | null): TagLayout {
    const availableWidth = Math.max(width, 1);
    const inputMinWidth = Math.min(TAG_INPUT_MIN_WIDTH, availableWidth);
    if (value.length === 0) {
        return {
            start: 0,
            end: 0,
            inputWidth: availableWidth,
            leadingWidth: 0,
            chipWidths: [],
        };
    }

    if (selectedTag !== null) {
        const start = Math.min(Math.max(selectedTag, 0), value.length - 1);
        const leadingWidth = start > 0 ? 2 : 0;
        const maxWidth = Math.max(availableWidth - leadingWidth - inputMinWidth - TAG_GAP, 1);
        const selectedWidth = Math.min(chipWidth(value[start] ?? "", availableWidth), maxWidth);
        return {
            start,
            end: start + 1,
            inputWidth: Math.max(availableWidth - leadingWidth - selectedWidth - TAG_GAP, 1),
            leadingWidth,
            chipWidths: [selectedWidth],
        };
    }

    let start = value.length;
    const widths: number[] = [];
    let chipTotal = 0;
    while (start > 0) {
        const candidateStart = start - 1;
        const candidateWidth = chipWidth(value[candidateStart] ?? "", availableWidth);
        const candidateTotal = chipTotal + (widths.length > 0 ? TAG_GAP : 0) + candidateWidth;
        const candidateLeading = candidateStart > 0 ? 2 : 0;
        if (candidateLeading + candidateTotal + TAG_GAP + inputMinWidth > availableWidth && widths.length > 0) {
            break;
        }
        widths.unshift(candidateWidth);
        chipTotal = candidateTotal;
        start = candidateStart;
    }

    if (widths.length === 0) {
        start = value.length - 1;
        const leadingWidth = start > 0 ? 2 : 0;
        const fallbackWidth = Math.max(availableWidth - leadingWidth - inputMinWidth - TAG_GAP, 1);
        widths.push(Math.min(chipWidth(value[start] ?? "", availableWidth), fallbackWidth));
        chipTotal = widths[0];
    }

    const leadingWidth = start > 0 ? 2 : 0;

    return {
        start,
        end: value.length,
        inputWidth: Math.max(availableWidth - leadingWidth - chipTotal - Math.max(widths.length - 1, 0) * TAG_GAP - TAG_GAP, 1),
        leadingWidth,
        chipWidths: widths,
    };
}

function sameStringArray(left: string[], right: string[]): boolean {
    return left.length === right.length && left.every((item, index) => item === right[index]);
}

export function StringListInput({
    value,
    width,
    active = true,
    onChange,
    onActivate,
    onMoveOutside,
    onClose,
}: StringListInputProps) {
    const [draft, setDraft] = useState("");
    const [selectedTag, setSelectedTag] = useState<number | null>(null);
    const [displayValue, setDisplayValue] = useState(value);
    const valueRef = useRef(value);
    const propValueRef = useRef(value);
    const draftRef = useRef(draft);
    const selectedTagRef = useRef(selectedTag);
    const inputRef = useRef<InputRenderable | null>(null);

    useEffect(() => {
        if (sameStringArray(value, valueRef.current)) {
            propValueRef.current = value;
            return;
        }
        if (sameStringArray(value, propValueRef.current)) {
            return;
        }
        propValueRef.current = value;
        valueRef.current = value;
        setDisplayValue(value);
    }, [value]);

    useEffect(() => {
        setSelectedTag((current) => {
            const next = current === null || displayValue.length === 0
                ? null
                : Math.min(current, displayValue.length - 1);
            selectedTagRef.current = next;
            return next;
        });
    }, [displayValue.length]);

    const activate = () => {
        onActivate?.();
    };

    const selectTag = (index: number | null) => {
        selectedTagRef.current = index;
        setSelectedTag(index);
        activate();
    };

    const removeTag = (requestedIndex: number | null) => {
        const current = valueRef.current;
        const index = requestedIndex ?? current.length - 1;
        if (index < 0 || index >= current.length) {
            return;
        }
        const next = current.filter((_item, itemIndex) => itemIndex !== index);
        valueRef.current = next;
        setDisplayValue(next);
        onChange(next);
        const nextSelection = next.length > 0 ? Math.min(Math.max(index - 1, 0), next.length - 1) : null;
        selectedTagRef.current = nextSelection;
        setSelectedTag(nextSelection);
    };

    const commitDraft = () => {
        const nextValue = draftRef.current;
        if (nextValue.length === 0) {
            selectedTagRef.current = null;
            setSelectedTag(null);
            return;
        }
        const next = [...valueRef.current, nextValue];
        valueRef.current = next;
        draftRef.current = "";
        setDraft("");
        selectedTagRef.current = null;
        setSelectedTag(null);
        setDisplayValue(next);
        if (inputRef.current) {
            inputRef.current.value = "";
        }
        onChange(next);
    };

    const handleKeyDown = (event: KeyEvent) => {
        if (!active) {
            return;
        }
        const currentDraft = draftRef.current;
        const currentSelection = selectedTagRef.current;

        if (event.name === "up") {
            event.preventDefault();
            event.stopPropagation();
            onMoveOutside?.(-1);
            return;
        }
        if (event.name === "down" || event.name === "tab") {
            event.preventDefault();
            event.stopPropagation();
            onMoveOutside?.(1);
            return;
        }
        if (event.name === "escape") {
            event.preventDefault();
            event.stopPropagation();
            onClose?.();
            return;
        }
        if (event.name === "return" || event.name === "linefeed") {
            event.preventDefault();
            event.stopPropagation();
            commitDraft();
            return;
        }
        if (event.name === "backspace" && currentDraft.length === 0) {
            event.preventDefault();
            event.stopPropagation();
            removeTag(currentSelection);
            return;
        }
        if (event.name === "left" && currentDraft.length === 0) {
            if (currentSelection === null) {
                if (valueRef.current.length > 0) {
                    event.preventDefault();
                    event.stopPropagation();
                    selectedTagRef.current = valueRef.current.length - 1;
                    setSelectedTag(valueRef.current.length - 1);
                }
            } else if (currentSelection > 0) {
                event.preventDefault();
                event.stopPropagation();
                selectedTagRef.current = currentSelection - 1;
                setSelectedTag(currentSelection - 1);
            }
            return;
        }
        if (event.name === "right" && currentDraft.length === 0 && currentSelection !== null) {
            event.preventDefault();
            event.stopPropagation();
            const nextSelection = currentSelection < valueRef.current.length - 1 ? currentSelection + 1 : null;
            selectedTagRef.current = nextSelection;
            setSelectedTag(nextSelection);
        }
    };

    useInput((_input, key, event) => {
        if (!active) {
            return;
        }
        if (
            key.upArrow ||
            key.downArrow ||
            key.tab ||
            key.escape ||
            key.return ||
            key.backspace ||
            (key.leftArrow && draftRef.current.length === 0) ||
            (key.rightArrow && draftRef.current.length === 0 && selectedTagRef.current !== null)
        ) {
            handleKeyDown(event);
        }
    }, { isActive: active });

    const layout = layoutTags(displayValue, width, selectedTag);
    const visibleTags = displayValue.slice(layout.start, layout.end);
    const tagLine = `${layout.leadingWidth > 0 ? "…" : ""}${visibleTags.map((item, index) => {
        const itemWidth = layout.chipWidths[index] ?? 1;
        const tagIndex = layout.start + index;
        const label = tagIndex === selectedTag ? `⟦${item}⟧` : `[${item}]`;
        return truncateText(label, itemWidth);
    }).join(" ")}`;
    return (
        <Box
            width={Math.max(width, 1)}
            height={1}
            overflow="hidden"
            alignItems="center"
        >
            <Box
                width={Math.max(width - layout.inputWidth, 0)}
                height={1}
                overflow="hidden"
            >
                <text
                    content={tagLine}
                    fg={theme.textMuted}
                    wrapMode="none"
                    truncate
                />
            </Box>
            <Box width={layout.inputWidth} height={1} overflow="hidden">
                <TextInput
                    ref={inputRef}
                    value={draft}
                    onChange={(next) => {
                        draftRef.current = next;
                        setDraft(next);
                        if (next.length > 0) {
                            selectedTagRef.current = null;
                            setSelectedTag(null);
                        }
                    }}
                    onSubmit={commitDraft}
                    placeholder={active ? "type + Enter" : ""}
                    focus={active}
                    onMouseDown={() => selectTag(null)}
                    onKeyDown={handleKeyDown}
                />
            </Box>
        </Box>
    );
}
