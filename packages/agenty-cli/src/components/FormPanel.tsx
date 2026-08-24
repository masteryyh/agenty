import type { InputRenderable, KeyEvent } from "@opentui/core";
import { useCallback, useMemo, useRef, useState } from "react";

import type { InputKey } from "../hooks/useInput";
import { useInput } from "../hooks/useInput";
import { useBottomDialogSize } from "./BottomDialog";
import { Panel } from "./Panel";
import { allocateColumnWidths, textWidth, truncateText } from "./Table";
import { ActionBar, Box, Pressable, Text, TextInput } from "./ui";

export interface FormOption {
    label: string;
    value: string;
}

export interface FormField {
    key: string;
    label: string;
    kind: "text" | "select" | "boolean" | "multiselect";
    value: string;
    options?: FormOption[];
    placeholder?: string;
    secret?: boolean;
    readOnly?: boolean;
    focusable?: boolean;
    visible?: boolean;
}

export interface FormAction {
    key: string;
    label: string;
}

export interface FormPanelProps {
    title: string;
    fields: FormField[];
    actions?: FormAction[];
    active?: boolean;
    error?: string | null;
    hint?: string;
    shortcutHint?: string;
    onChange?: (key: string, allValues: Record<string, string>) => void;
    onShortcut?: (
        input: string,
        key: InputKey,
        event: KeyEvent,
        values: Record<string, string>,
    ) => boolean;
    onAction: (key: string, values: Record<string, string>) => void;
    onClose: () => void;
}

function maskValue(value: string): string {
    if (!value) {
        return "—";
    }
    return "•".repeat(Math.min(value.length, 20));
}

function selectLabel(options: FormOption[], value: string): string {
    return options.find((option) => option.value === value)?.label ?? value;
}

function parseMulti(value: string): Set<string> {
    try {
        const parsed: unknown = JSON.parse(value);
        if (Array.isArray(parsed)) {
            return new Set(parsed.filter((item): item is string => typeof item === "string"));
        }
    } catch {
        return new Set();
    }
    return new Set();
}

function serializeMulti(values: Set<string>): string {
    return JSON.stringify(Array.from(values));
}

type ChoiceState =
    | { kind: "idle" }
    | { kind: "selecting"; visibleIndex: number; selection: number }
    | {
        kind: "multi-selecting";
        visibleIndex: number;
        selection: number;
        chosen: Set<string>;
    };

export function FormPanel({
    title,
    fields,
    actions,
    active = true,
    error,
    hint: hintOverride,
    shortcutHint,
    onChange,
    onShortcut,
    onAction,
    onClose,
}: FormPanelProps) {
    const dialogSize = useBottomDialogSize();
    const visibleFields = useMemo(
        () => fields.filter((field) => field.visible !== false),
        [fields],
    );
    const actionDefs = useMemo<FormAction[]>(
        () => actions ?? [
            { key: "save", label: "Save" },
            { key: "cancel", label: "Cancel" },
        ],
        [actions],
    );
    const actionStart = visibleFields.length;
    const actionEnd = actionStart + actionDefs.length - 1;
    const navigationIndexes = useMemo(() => [
        ...visibleFields.flatMap((field, index) => field.focusable === false ? [] : [index]),
        ...actionDefs.map((_action, index) => actionStart + index),
    ], [actionDefs, actionStart, visibleFields]);
    const [values, setValues] = useState<Record<string, string>>(() =>
        Object.fromEntries(fields.map((field) => [field.key, field.value])),
    );
    const [cursor, setCursor] = useState(navigationIndexes[0] ?? 0);
    const [choice, setChoice] = useState<ChoiceState>({ kind: "idle" });
    const textInputRef = useRef<InputRenderable | null>(null);

    const formColumnBudget = Math.max(dialogSize.width - 3, 0);
    const labelContentWidth = Math.max(
        ...visibleFields.map((field) => textWidth(`${field.label}:`)),
        0,
    );
    const [keyWidth = 0] = allocateColumnWidths(
        formColumnBudget,
        [labelContentWidth, formColumnBudget],
    );
    const maxExpandedOptions = Math.max(
        2,
        Math.min(6, dialogSize.height - visibleFields.length - 4),
    );
    const valuesRef = useRef(values);
    valuesRef.current = values;
    const cursorRef = useRef(cursor);
    cursorRef.current = cursor;
    const choiceRef = useRef(choice);
    choiceRef.current = choice;
    const visibleFieldsRef = useRef(visibleFields);
    visibleFieldsRef.current = visibleFields;
    const actionDefsRef = useRef(actionDefs);
    actionDefsRef.current = actionDefs;

    const updateValue = useCallback((key: string, value: string) => {
        const next = { ...valuesRef.current, [key]: value };
        valuesRef.current = next;
        setValues(next);
        if (onChange) {
            queueMicrotask(() => onChange(key, next));
        }
    }, [onChange]);

    const moveCursor = useCallback((direction: -1 | 1, from = cursorRef.current) => {
        const currentPosition = navigationIndexes.indexOf(from);
        const fallbackPosition = direction > 0 ? -1 : navigationIndexes.length;
        const nextPosition = Math.min(
            Math.max(currentPosition < 0 ? fallbackPosition + direction : currentPosition + direction, 0),
            Math.max(navigationIndexes.length - 1, 0),
        );
        const next = navigationIndexes[nextPosition];
        if (next === undefined) {
            return;
        }
        textInputRef.current?.blur();
        setChoice({ kind: "idle" });
        setCursor(next);
    }, [navigationIndexes]);

    const openChoice = useCallback((visibleIndex: number) => {
        const field = visibleFieldsRef.current[visibleIndex];
        if (!field || field.readOnly) {
            return;
        }
        const options = field.options ?? [];
        if (options.length === 0) {
            return;
        }

        const current = valuesRef.current[field.key] ?? field.value;
        if (field.kind === "select") {
            const selected = options.findIndex((option) => option.value === current);
            setChoice({
                kind: "selecting",
                visibleIndex,
                selection: selected >= 0 ? selected : 0,
            });
        } else if (field.kind === "multiselect") {
            const chosen = parseMulti(current);
            const firstChosen = options.findIndex((option) => chosen.has(option.value));
            setChoice({
                kind: "multi-selecting",
                visibleIndex,
                selection: firstChosen >= 0 ? firstChosen : 0,
                chosen,
            });
        }
    }, []);

    const commitSelect = useCallback((selection: number) => {
        const state = choiceRef.current;
        if (state.kind !== "selecting") {
            return;
        }
        const field = visibleFieldsRef.current[state.visibleIndex];
        const option = field?.options?.[selection];
        if (field && option) {
            updateValue(field.key, option.value);
        }
        setChoice({ kind: "idle" });
    }, [updateValue]);

    const toggleMultiSelect = useCallback((selection: number) => {
        setChoice((state) => {
            if (state.kind !== "multi-selecting") {
                return state;
            }
            const option = visibleFieldsRef.current[state.visibleIndex]?.options?.[selection];
            if (!option) {
                return state;
            }
            const chosen = new Set(state.chosen);
            if (chosen.has(option.value)) {
                chosen.delete(option.value);
            } else {
                chosen.add(option.value);
            }
            return { ...state, selection, chosen };
        });
    }, []);

    const commitMultiSelect = useCallback(() => {
        const state = choiceRef.current;
        if (state.kind !== "multi-selecting") {
            return;
        }
        const field = visibleFieldsRef.current[state.visibleIndex];
        if (field) {
            updateValue(field.key, serializeMulti(state.chosen));
        }
        setChoice({ kind: "idle" });
    }, [updateValue]);

    const runAction = useCallback((actionIndex: number) => {
        const action = actionDefsRef.current[actionIndex];
        if (!action) {
            return;
        }
        if (action.key === "cancel") {
            onClose();
        } else {
            onAction(action.key, valuesRef.current);
        }
    }, [onAction, onClose]);

    useInput((input, key, event) => {
        const state = choiceRef.current;
        if (state.kind === "selecting") {
            const options = visibleFieldsRef.current[state.visibleIndex]?.options ?? [];
            if (key.escape) {
                setChoice({ kind: "idle" });
            } else if (key.upArrow) {
                setChoice({ ...state, selection: Math.max(state.selection - 1, 0) });
            } else if (key.downArrow) {
                setChoice({
                    ...state,
                    selection: Math.min(state.selection + 1, Math.max(options.length - 1, 0)),
                });
            } else if (key.return) {
                commitSelect(state.selection);
            }
            return;
        }

        if (state.kind === "multi-selecting") {
            const options = visibleFieldsRef.current[state.visibleIndex]?.options ?? [];
            if (key.escape) {
                setChoice({ kind: "idle" });
            } else if (key.upArrow) {
                setChoice({ ...state, selection: Math.max(state.selection - 1, 0) });
            } else if (key.downArrow) {
                setChoice({
                    ...state,
                    selection: Math.min(state.selection + 1, Math.max(options.length - 1, 0)),
                });
            } else if (input === " ") {
                toggleMultiSelect(state.selection);
            } else if (key.return) {
                commitMultiSelect();
            }
            return;
        }

        const current = cursorRef.current;
        const field = visibleFieldsRef.current[current];
        const editingText = field?.kind === "text" && !field.readOnly;
        if (!editingText && onShortcut?.(input, key, event, valuesRef.current)) {
            return;
        }
        if (key.escape) {
            onClose();
            return;
        }
        if (key.upArrow) {
            event.preventDefault();
            moveCursor(-1);
            return;
        }
        if (key.downArrow || key.tab) {
            event.preventDefault();
            moveCursor(1);
            return;
        }

        if (current >= actionStart && current <= actionEnd) {
            if (key.leftArrow) {
                moveCursor(-1);
            } else if (key.rightArrow) {
                moveCursor(1);
            } else if (key.return) {
                runAction(current - actionStart);
            }
            return;
        }
        if (!field || field.focusable === false || field.readOnly || editingText) {
            return;
        }
        if (field.kind === "boolean") {
            if (key.leftArrow || key.rightArrow || key.return || input === " ") {
                const value = valuesRef.current[field.key] ?? field.value;
                updateValue(field.key, value === "true" ? "false" : "true");
            }
        } else if ((field.kind === "select" || field.kind === "multiselect") && key.return) {
            openChoice(current);
        }
    }, { isActive: active });

    const hint = hintOverride ?? (dialogSize.width < 60
        ? "↑↓ move · Enter choose · Esc back"
        : "↑↓ navigate · type to edit · Enter open/choose · Space toggle · Esc back");
    const choiceField = choice.kind === "idle"
        ? undefined
        : visibleFields[choice.visibleIndex];
    const choiceOptions = choiceField?.options ?? [];
    const choiceSelection = choice.kind === "idle" ? 0 : choice.selection;
    const choiceOptionStart = Math.max(
        0,
        Math.min(
            choiceSelection - Math.floor(maxExpandedOptions / 2),
            Math.max(choiceOptions.length - maxExpandedOptions, 0),
        ),
    );
    const choiceVisibleOptions = choiceOptions.slice(
        choiceOptionStart,
        choiceOptionStart + maxExpandedOptions,
    );

    return (
        <Panel
            title={title}
            error={error}
            footer={(
                <ActionBar
                    actions={actionDefs}
                    activeKey={cursor >= actionStart ? actionDefs[cursor - actionStart]?.key : undefined}
                    gap={3}
                    onAction={(key) => {
                        const index = actionDefs.findIndex((action) => action.key === key);
                        if (index >= 0) {
                            setCursor(actionStart + index);
                            runAction(index);
                        }
                    }}
                />
            )}
            hint={shortcutHint ? `${hint} · ${shortcutHint}` : hint}
        >
            <Box
                flexDirection="column"
                flexGrow={1}
                width="100%"
                position="relative"
                overflow="hidden"
            >
                {visibleFields.map((field, visibleIndex) => {
                    const selected = cursor === visibleIndex;
                    const value = values[field.key] ?? field.value;
                    const options = field.options ?? [];
                    const editingText = active && selected && field.kind === "text" && !field.readOnly;

                    return (
                        <Pressable
                            key={field.key}
                            width="100%"
                            height={1}
                            overflow="hidden"
                            disabled={!active || field.focusable === false}
                            onPress={() => {
                                if (field.focusable === false) {
                                    return;
                                }
                                setCursor(visibleIndex);
                                setChoice({ kind: "idle" });
                                if (field.kind === "boolean" && !field.readOnly) {
                                    updateValue(field.key, value === "true" ? "false" : "true");
                                }
                            }}
                        >
                            <Box width={2} height={1}>
                                <Text color={selected ? "cyan" : "gray"}>
                                    {selected ? "❯" : " "}
                                </Text>
                            </Box>
                            <Box
                                width={keyWidth}
                                height={1}
                                justifyContent="flex-end"
                                overflow="hidden"
                            >
                                <Text
                                    color={selected ? "cyan" : "gray"}
                                    bold={selected}
                                    wrap="truncate"
                                >
                                    {truncateText(`${field.label}:`, keyWidth)}
                                </Text>
                            </Box>
                            <Text> </Text>
                            <Box
                                flexGrow={1}
                                flexBasis={0}
                                height={1}
                                justifyContent="flex-start"
                                overflow="hidden"
                            >
                                {editingText ? (
                                    <TextInput
                                        ref={textInputRef}
                                        value={value}
                                        onChange={(next) => updateValue(field.key, next)}
                                        onSubmit={() => moveCursor(1, visibleIndex)}
                                        placeholder={field.placeholder ?? ""}
                                        focus={active}
                                        onKeyDown={(event) => {
                                            if (event.name === "up") {
                                                event.preventDefault();
                                                event.stopPropagation();
                                                moveCursor(-1, visibleIndex);
                                            } else if (event.name === "down" || event.name === "tab") {
                                                event.preventDefault();
                                                event.stopPropagation();
                                                moveCursor(1, visibleIndex);
                                            } else if (event.name === "escape") {
                                                event.preventDefault();
                                                event.stopPropagation();
                                                onClose();
                                            }
                                        }}
                                    />
                                ) : (
                                    <Text wrap="truncate" color={selected ? "cyan" : "white"}>
                                        {field.kind === "boolean"
                                            ? renderBoolean(selected, value)
                                            : field.kind === "select"
                                                ? selectLabel(options, value)
                                                : field.kind === "multiselect"
                                                    ? renderMultiValue(value)
                                                    : field.secret
                                                        ? maskValue(value)
                                                        : value || <Text dimColor>—</Text>}
                                    </Text>
                                )}
                            </Box>
                        </Pressable>
                    );
                })}
                {choice.kind === "idle" ? null : (
                    <Box
                        key={`choice-menu:${choice.kind}`}
                        position="absolute"
                        top={choice.visibleIndex + 1}
                        left={0}
                        right={0}
                        height={maxExpandedOptions}
                        zIndex={10}
                        overflow="hidden"
                    >
                        <Box width={keyWidth + 3} height={maxExpandedOptions} />
                        <Box
                            flexDirection="column"
                            flexGrow={1}
                            flexBasis={0}
                            height={maxExpandedOptions}
                            backgroundColor="#101417"
                        >
                            {Array.from({ length: maxExpandedOptions }, (_, localIndex) => {
                                const option = choiceVisibleOptions[localIndex];
                                const index = choiceOptionStart + localIndex;
                                const activeOption = option !== undefined && choice.selection === index;
                                const checked = option !== undefined && choice.kind === "multi-selecting" &&
                                    choice.chosen.has(option.value);
                                return (
                                    <Pressable
                                        key={localIndex}
                                        width="100%"
                                        height={1}
                                        disabled={!option}
                                        onPress={() => {
                                            if (!option) {
                                                return;
                                            }
                                            if (choice.kind === "selecting") {
                                                commitSelect(index);
                                            } else {
                                                toggleMultiSelect(index);
                                            }
                                        }}
                                    >
                                        <Text color={activeOption ? "cyan" : "gray"}>
                                            {option && activeOption ? "❯ " : "  "}
                                        </Text>
                                        <Text color={checked ? "cyan" : activeOption ? "cyan" : "white"} bold={checked || activeOption}>
                                            {choice.kind === "multi-selecting"
                                                ? `${checked ? "✓" : "☐"} ${option?.label ?? ""}`
                                                : option?.label ?? ""}
                                        </Text>
                                    </Pressable>
                                );
                            })}
                        </Box>
                    </Box>
                )}
            </Box>
        </Panel>
    );
}

function renderBoolean(selected: boolean, value: string): React.ReactNode {
    const enabled = value === "true";
    return (
        <Text color={selected && enabled ? "cyan" : "gray"} bold={selected && enabled}>
            {enabled ? "◉ true" : "○ false"}
        </Text>
    );
}

function renderMultiValue(value: string): React.ReactNode {
    const chosen = parseMulti(value);
    if (chosen.size === 0) {
        return <Text dimColor>—</Text>;
    }
    return <Text>{`${chosen.size} selected`}</Text>;
}
