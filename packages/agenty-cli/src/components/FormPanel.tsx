import type { InputRenderable, KeyEvent } from "@opentui/core";
import { useCallback, useMemo, useRef, useState } from "react";

import type { InputKey } from "../hooks/useInput";
import { useInput } from "../hooks/useInput";
import { useBottomDialogSize } from "./BottomDialog";
import { dropdownFieldMode, DropdownMenu, dropdownValueForField } from "./DropdownMenu";
import { Panel } from "./Panel";
import { textWidth, truncateText } from "./Table";
import { ActionBar, Box, Pressable, Text, TextInput } from "./ui";

const FORM_LABEL_MAX_WIDTH = 24;
const FORM_VALUE_WIDTH = 48;
const FORM_MIN_VALUE_WIDTH = 12;
const FORM_COLUMN_GAP = 2;
const FORM_MENU_BORDER_HEIGHT = 2;
const FORM_MENU_MIN_ROWS = 4;
const FORM_MENU_MAX_ROWS = 8;

export interface FormOption {
    label: string;
    value: string;
}

export interface FormField {
    key: string;
    label: string;
    kind: "text" | "select" | "boolean" | "multiselect" | "disclosure";
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

export interface DropdownPlacement {
    side: "above" | "below";
    visibleRows: number;
    height: number;
}

export function chooseDropdownPlacement(
    fieldTop: number,
    fieldHeight: number,
    viewportHeight: number,
    optionCount: number,
    maxRows: number,
): DropdownPlacement {
    const boundedMaxRows = Math.max(Math.min(maxRows, optionCount), 1);
    const belowSpace = Math.max(viewportHeight - fieldTop - fieldHeight, 0);
    const aboveSpace = Math.max(fieldTop, 0);
    const fullHeight = boundedMaxRows + FORM_MENU_BORDER_HEIGHT;
    const side = belowSpace >= fullHeight
        ? "below"
        : aboveSpace >= fullHeight
            ? "above"
            : belowSpace >= aboveSpace
                ? "below"
                : "above";
    const availableSpace = side === "below" ? belowSpace : aboveSpace;
    const visibleRows = Math.max(
        Math.min(boundedMaxRows, Math.max(availableSpace - FORM_MENU_BORDER_HEIGHT, 1)),
        1,
    );

    return {
        side,
        visibleRows,
        height: visibleRows + FORM_MENU_BORDER_HEIGHT,
    };
}

export function preferredDropdownRows(viewportHeight: number): number {
    return Math.max(
        FORM_MENU_MIN_ROWS,
        Math.min(FORM_MENU_MAX_ROWS, Math.floor(viewportHeight * 0.4)),
    );
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

function splitWord(word: string, width: number): string[] {
    const chunks: string[] = [];
    let chunk = "";
    let chunkWidth = 0;

    for (const character of word) {
        const characterWidth = textWidth(character);
        if (chunk && chunkWidth + characterWidth > width) {
            chunks.push(chunk);
            chunk = "";
            chunkWidth = 0;
        }
        chunk += character;
        chunkWidth += characterWidth;
    }
    if (chunk) {
        chunks.push(chunk);
    }

    return chunks;
}

export function wrapFormLabel(value: string, width: number): string[] {
    if (width <= 0) {
        return [""];
    }

    const lines: string[] = [];
    let current = "";
    for (const word of value.trim().split(/\s+/)) {
        const chunks = textWidth(word) > width ? splitWord(word, width) : [word];
        for (const chunk of chunks) {
            const candidate = current ? `${current} ${chunk}` : chunk;
            if (textWidth(candidate) <= width) {
                current = candidate;
                continue;
            }
            if (current) {
                lines.push(current);
            }
            current = chunk;
        }
    }
    if (current) {
        lines.push(current);
    }

    return lines.length > 0 ? lines : [""];
}

type ChoiceState =
    | { kind: "idle" }
    | { kind: "open"; visibleIndex: number };

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

    const hasMeasuredDialog = dialogSize.width > 1;
    const formColumnBudget = Math.max(dialogSize.width - 2, 1);
    const labelContentWidth = Math.max(
        ...fields
            .filter((field) => field.kind !== "disclosure")
            .map((field) => textWidth(`${field.label}:`)),
        1,
    );
    const preferredLabelWidth = Math.min(labelContentWidth, FORM_LABEL_MAX_WIDTH);
    const labelWidth = hasMeasuredDialog
        ? Math.min(
            preferredLabelWidth,
            Math.max(formColumnBudget - FORM_MIN_VALUE_WIDTH - FORM_COLUMN_GAP, 1),
        )
        : preferredLabelWidth;
    const valueWidth = hasMeasuredDialog
        ? Math.max(
            Math.min(FORM_VALUE_WIDTH, formColumnBudget - labelWidth - FORM_COLUMN_GAP),
            1,
        )
        : undefined;
    const fieldLayouts = visibleFields.map((field) => {
        const labelLines = field.kind === "disclosure"
            ? [field.label]
            : wrapFormLabel(`${field.label}:`, labelWidth);
        return {
            labelLines,
            height: field.kind === "disclosure" ? 1 : labelLines.length,
        };
    });
    const menuRowBudget = hasMeasuredDialog
        ? preferredDropdownRows(Math.max(dialogSize.height - 4, 1))
        : 6;
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

        if (field.kind === "select" || field.kind === "multiselect") {
            setChoice({ kind: "open", visibleIndex });
        }
    }, []);

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
        if (state.kind === "open") {
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
        if (!field || field.focusable === false || editingText) {
            return;
        }
        if (field.kind === "disclosure") {
            const expanded = (valuesRef.current[field.key] ?? field.value) === "true";
            if (key.leftArrow && expanded) {
                updateValue(field.key, "false");
            } else if (key.rightArrow && !expanded) {
                updateValue(field.key, "true");
            } else if (key.return || input === " ") {
                updateValue(field.key, expanded ? "false" : "true");
            }
        } else if (field.readOnly) {
            return;
        } else if (field.kind === "boolean") {
            if (key.leftArrow || key.rightArrow || key.return || input === " ") {
                const value = valuesRef.current[field.key] ?? field.value;
                updateValue(field.key, value === "true" ? "false" : "true");
            }
        } else if ((field.kind === "select" || field.kind === "multiselect") && key.return) {
            openChoice(current);
        }
    }, { isActive: active });

    const hasDisclosure = visibleFields.some((field) => field.kind === "disclosure");
    const hint = hintOverride ?? (dialogSize.width < 60
        ? hasDisclosure
            ? "↑↓ move · ←→ expand/collapse · Enter choose · Esc back"
            : "↑↓ move · Enter choose · Esc back"
        : hasDisclosure
            ? "↑↓ navigate · ←→ expand/collapse · type to edit · Enter open/choose · Space toggle · Esc back"
            : "↑↓ navigate · type to edit · Enter open/choose · Space toggle · Esc back");
    const choiceField = choice.kind === "idle"
        ? undefined
        : visibleFields[choice.visibleIndex];
    const choiceFieldTop = choiceField && choice.kind !== "idle"
        ? fieldLayouts
            .slice(0, choice.visibleIndex)
            .reduce((total, layout) => total + layout.height, 0)
        : 0;
    const choiceIndex = choice.kind === "open" ? choice.visibleIndex : 0;
    const dropdownPlacement = choiceField && choice.kind !== "idle"
        ? hasMeasuredDialog
            ? chooseDropdownPlacement(
                choiceFieldTop,
                fieldLayouts[choice.visibleIndex]?.height ?? 1,
                Math.max(dialogSize.height - 4, 1),
                choiceField.options?.length ?? 0,
                menuRowBudget,
            )
            : {
                side: "below" as const,
                visibleRows: menuRowBudget,
                height: menuRowBudget + FORM_MENU_BORDER_HEIGHT,
            }
        : undefined;

    return (
        <Panel
            title={title}
            error={error}
            contentOverflow="visible"
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
                overflow="visible"
            >
                {visibleFields.map((field, visibleIndex) => {
                    const selected = cursor === visibleIndex;
                    const value = values[field.key] ?? field.value;
                    const options = field.options ?? [];
                    const editingText = active && selected && field.kind === "text" && !field.readOnly;
                    const layout = fieldLayouts[visibleIndex];
                    const rowHeight = layout?.height ?? 1;
                    const labelLines = layout?.labelLines ?? [field.label];
                    const choiceOpen = choice.kind !== "idle" && choice.visibleIndex === visibleIndex;

                    return (
                        <Pressable
                            key={field.key}
                            width="100%"
                            height={rowHeight}
                            position="relative"
                            overflow="visible"
                            alignItems="flex-start"
                            disabled={!active || field.focusable === false}
                            onPress={() => {
                                if (field.focusable === false) {
                                    return;
                                }
                                setCursor(visibleIndex);
                                setChoice({ kind: "idle" });
                                if (field.kind === "disclosure") {
                                    updateValue(field.key, value === "true" ? "false" : "true");
                                } else if (field.kind === "boolean" && !field.readOnly) {
                                    updateValue(field.key, value === "true" ? "false" : "true");
                                }
                            }}
                        >
                            <Box width={2} height={rowHeight}>
                                <Text color={selected ? "cyan" : "gray"}>
                                    {selected ? "❯" : " "}
                                </Text>
                            </Box>
                            {field.kind === "disclosure" ? (
                                <Box
                                    flexGrow={1}
                                    flexBasis={0}
                                    height={rowHeight}
                                    justifyContent={hasMeasuredDialog ? "space-around" : "flex-start"}
                                    alignItems="flex-start"
                                    overflow="hidden"
                                >
                                    <Box
                                        width={labelWidth}
                                        height={1}
                                        justifyContent="flex-end"
                                        overflow="hidden"
                                    >
                                        <Text color={selected ? "cyan" : "gray"} bold={selected}>
                                            {`${value === "true" ? "▾" : "▸"} ${field.label}`}
                                        </Text>
                                    </Box>
                                    <Box width={valueWidth} height={1} flexShrink={0} />
                                </Box>
                            ) : (
                                <Box
                                    flexGrow={1}
                                    flexBasis={0}
                                    height={rowHeight}
                                    justifyContent={hasMeasuredDialog ? "space-around" : "flex-start"}
                                    alignItems="flex-start"
                                    overflow="hidden"
                                >
                                    <Box
                                        width={labelWidth}
                                        height={rowHeight}
                                        flexDirection="column"
                                        overflow="hidden"
                                    >
                                        {labelLines.map((line, lineIndex) => (
                                            <Box
                                                key={`${field.key}:label:${lineIndex}`}
                                                width={labelWidth}
                                                height={1}
                                                justifyContent="flex-end"
                                                overflow="hidden"
                                            >
                                                <Text color={selected ? "cyan" : "gray"} bold={selected}>
                                                    {line}
                                                </Text>
                                            </Box>
                                        ))}
                                    </Box>
                                    <Box
                                        width={valueWidth}
                                        height={1}
                                        flexGrow={valueWidth === undefined ? 1 : 0}
                                        flexShrink={valueWidth === undefined ? 1 : 0}
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
                                            <Text color={selected ? "cyan" : "white"}>
                                                {field.kind === "boolean"
                                                    ? renderBoolean(selected, value)
                                                    : field.kind === "multiselect"
                                                        ? renderMultiValue(value)
                                                        : renderTextValue(field, value, options, valueWidth ?? 48)}
                                            </Text>
                                        )}
                                    </Box>
                                </Box>
                            )}
                        </Pressable>
                    );
                })}
                {choiceField && dropdownPlacement ? (
                    <Box
                        position="absolute"
                        top={dropdownPlacement.side === "above"
                            ? choiceFieldTop - dropdownPlacement.height
                            : choiceFieldTop + (fieldLayouts[choiceIndex]?.height ?? 1)}
                        left={0}
                        right={0}
                        height={dropdownPlacement.height}
                        zIndex={10}
                        overflow="hidden"
                    >
                        <Box width={2} height={dropdownPlacement.height} />
                        <Box
                            flexGrow={1}
                            flexBasis={0}
                            height={dropdownPlacement.height}
                            justifyContent={hasMeasuredDialog ? "space-around" : "flex-start"}
                            alignItems="flex-start"
                        >
                            <Box width={labelWidth} height={dropdownPlacement.height} />
                            <DropdownMenu
                                options={choiceField.options ?? []}
                                mode={dropdownFieldMode(choiceField.kind === "select" ? "select" : "multiselect")}
                                value={dropdownValueForField(
                                    dropdownFieldMode(choiceField.kind === "select" ? "select" : "multiselect"),
                                    values[choiceField.key] ?? choiceField.value,
                                )}
                                width={valueWidth ?? 48}
                                maxVisible={dropdownPlacement.visibleRows}
                                onSubmit={(next) => {
                                    const submitted = Array.isArray(next) ? serializeMulti(new Set(next)) : next;
                                    updateValue(choiceField.key, submitted);
                                    setChoice({ kind: "idle" });
                                }}
                                onClose={() => setChoice({ kind: "idle" })}
                            />
                        </Box>
                    </Box>
                ) : null}
            </Box>
        </Panel>
    );
}

function renderTextValue(
    field: FormField,
    value: string,
    options: FormOption[],
    width: number,
): React.ReactNode {
    const displayValue = field.kind === "select"
        ? selectLabel(options, value)
        : field.secret
            ? maskValue(value)
            : value;
    if (!displayValue) {
        return <Text dimColor>—</Text>;
    }
    return truncateText(displayValue, width);
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
