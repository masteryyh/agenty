import { useRef, useState } from "react";

import { useInput } from "../hooks/useInput";
import { Box, Pressable, Text } from "./ui";

export interface DropdownMenuOption {
    label: string;
    value: string;
}

export type DropdownMenuMode = "single" | "multiple";

export interface DropdownMenuProps {
    options: DropdownMenuOption[];
    mode: DropdownMenuMode;
    value: string | string[];
    width: number;
    maxVisible?: number;
    onSubmit: (value: string | string[]) => void;
    onClose: () => void;
}

function selectedValues(value: string | string[]): Set<string> {
    return new Set(Array.isArray(value) ? value : [value]);
}

export function DropdownMenu({
    options,
    mode,
    value,
    width,
    maxVisible = 6,
    onSubmit,
    onClose,
}: DropdownMenuProps) {
    const visibleCount = Math.max(Math.min(maxVisible, options.length), 1);
    const initialSelection = mode === "single"
        ? Math.max(options.findIndex((option) => option.value === value), 0)
        : Math.max(options.findIndex((option) => selectedValues(value).has(option.value)), 0);
    const [selection, setSelection] = useState(initialSelection);
    const [chosen, setChosen] = useState(() => selectedValues(value));
    const selectionRef = useRef(selection);
    const chosenRef = useRef(chosen);
    const optionsRef = useRef(options);
    selectionRef.current = selection;
    chosenRef.current = chosen;
    optionsRef.current = options;

    const submit = (index: number) => {
        const option = optionsRef.current[index];
        if (!option) {
            return;
        }
        if (mode === "single") {
            onSubmit(option.value);
        } else {
            onSubmit(optionsRef.current
                .filter((candidate) => chosenRef.current.has(candidate.value))
                .map((candidate) => candidate.value));
        }
    };

    useInput((input, key, event) => {
        if (key.escape) {
            onClose();
            return;
        }
        if (key.upArrow) {
            event.preventDefault();
            setSelection((current) => Math.max(current - 1, 0));
            return;
        }
        if (key.downArrow) {
            event.preventDefault();
            setSelection((current) => Math.min(current + 1, optionsRef.current.length - 1));
            return;
        }
        if (mode === "multiple" && input === " ") {
            const option = optionsRef.current[selectionRef.current];
            if (option) {
                setChosen((current) => {
                    const next = new Set(current);
                    if (next.has(option.value)) {
                        next.delete(option.value);
                    } else {
                        next.add(option.value);
                    }
                    chosenRef.current = next;
                    return next;
                });
            }
            return;
        }
        if (key.return) {
            submit(selectionRef.current);
        }
    }, { isActive: true });

    const start = Math.max(
        0,
        Math.min(
            selection - Math.floor(visibleCount / 2),
            Math.max(options.length - visibleCount, 0),
        ),
    );
    const visibleOptions = options.slice(start, start + visibleCount);
    const panelHeight = visibleCount + 2;

    return (
        <Box
            width={Math.max(width, 1)}
            height={panelHeight}
            flexDirection="column"
            borderStyle="single"
            borderColor="#405158"
            backgroundColor="#101417"
            overflow="hidden"
        >
            {visibleOptions.map((option, localIndex) => {
                const index = start + localIndex;
                const active = selection === index;
                const checked = mode === "multiple" && chosen.has(option.value);
                return (
                    <Pressable
                        key={option.value}
                        width="100%"
                        height={1}
                        onPress={() => {
                            setSelection(index);
                            if (mode === "single") {
                                onSubmit(option.value);
                            } else {
                                setChosen((current) => {
                                    const next = new Set(current);
                                    if (next.has(option.value)) {
                                        next.delete(option.value);
                                    } else {
                                        next.add(option.value);
                                    }
                                    chosenRef.current = next;
                                    return next;
                                });
                            }
                        }}
                    >
                        <Text color={active ? "cyan" : "gray"}>
                            {active ? "❯ " : "  "}
                        </Text>
                        <Text
                            color={active || checked ? "cyan" : "white"}
                            bold={active || checked}
                            wrap="truncate"
                        >
                            {mode === "multiple" ? `${checked ? "✓" : "☐"} ${option.label}` : option.label}
                        </Text>
                    </Pressable>
                );
            })}
        </Box>
    );
}

export function dropdownValueForField(
    mode: DropdownMenuMode,
    value: string,
): string | string[] {
    if (mode === "single") {
        return value;
    }
    try {
        const parsed: unknown = JSON.parse(value || "[]");
        return Array.isArray(parsed)
            ? parsed.filter((item): item is string => typeof item === "string")
            : [];
    } catch {
        return [];
    }
}

export function dropdownFieldMode(kind: "select" | "multiselect"): DropdownMenuMode {
    return kind === "select" ? "single" : "multiple";
}
