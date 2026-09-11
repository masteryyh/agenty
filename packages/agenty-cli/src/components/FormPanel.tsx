import type { InputRenderable, KeyEvent, ScrollBoxRenderable } from "@opentui/core";
import type { ReactNode } from "react";
import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";

import type { InputKey } from "../hooks/useInput";
import { useInput } from "../hooks/useInput";
import { useWindowSize } from "../hooks/useWindowSize";
import { useBottomDialogSize } from "./BottomDialog";
import { dropdownFieldMode, DropdownMenu, dropdownValueForField } from "./DropdownMenu";
import { FormFieldRow } from "./FormFieldRow";
import { layoutFormActions, resolveFormLayout, wrapFormLabel } from "./formLayout";
import { FormShell } from "./FormShell";
import { StringListInput } from "./StringListInput";
import { truncateText } from "./Table";
import { ActionBar, Box, Text, TextInput } from "./ui";

export { wrapFormLabel } from "./formLayout";

const FORM_MENU_BORDER_HEIGHT = 2;
const FORM_MENU_MIN_ROWS = 4;
const FORM_MENU_MAX_ROWS = 8;

export interface FormOption {
    label: string;
    value: string;
}

interface ScalarFormField {
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

export interface StringListFormField {
    key: string;
    label: string;
    kind: "string-list";
    value: string[];
    readOnly?: boolean;
    focusable?: boolean;
    visible?: boolean;
}

export type FormField = ScalarFormField | StringListFormField;
export type FormValue = string | string[];
export type FormValues = Record<string, FormValue>;

export interface FormAction {
    key: string;
    label: string;
}

export interface FormPanelProps {
    title: string;
    fields: FormField[];
    actions?: FormAction[];
    afterFields?: ReactNode;
    active?: boolean;
    error?: string | null;
    hint?: string;
    shortcutHint?: string;
    onChange?: (key: string, allValues: FormValues) => void;
    onShortcut?: (
        input: string,
        key: InputKey,
        event: KeyEvent,
        values: FormValues,
    ) => boolean;
    onAction: (key: string, values: FormValues) => void;
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

export function formString(values: FormValues, key: string): string {
    const value = values[key];
    return typeof value === "string" ? value : "";
}

export function formStringList(values: FormValues, key: string): string[] {
    const value = values[key];
    return Array.isArray(value) ? value : [];
}

type ChoiceState =
    | { kind: "idle" }
    | { kind: "open"; visibleIndex: number };

export function FormPanel({
    title,
    fields,
    actions,
    afterFields,
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
    const terminal = useWindowSize();
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
    const [values, setValues] = useState<FormValues>(() =>
        Object.fromEntries(fields.map((field) => [field.key, field.value])),
    );
    const focusKeys = [
        ...visibleFields.map((field) => `field:${field.key}`),
        ...actionDefs.map((action) => `action:${action.key}`),
    ];
    const [focusKey, setFocusKey] = useState(focusKeys[navigationIndexes[0] ?? 0]);
    const focusIndex = focusKeys.indexOf(focusKey ?? "");
    const cursor = navigationIndexes.includes(focusIndex) ? focusIndex : navigationIndexes[0] ?? 0;
    const setCursor = (index: number) => setFocusKey(focusKeys[index]);
    const [choice, setChoice] = useState<ChoiceState>({ kind: "idle" });
    const textInputRef = useRef<InputRenderable | null>(null);

    const width = dialogSize.width > 1 ? dialogSize.width : terminal.columns;
    const height = dialogSize.width > 1 ? dialogSize.height : terminal.rows;
    const layout = resolveFormLayout(width, fields.filter((field) => field.kind !== "disclosure").map((field) => field.label));
    const fieldLayouts = visibleFields.map((field) => {
        const labelLines = wrapFormLabel(`${field.label}:`, layout.labelWidth);
        const valueTop = layout.mode === "stacked" ? labelLines.length : 0;
        return {
            labelLines,
            valueTop,
            height: field.kind === "disclosure" ? 1 : Math.max(labelLines.length, valueTop + 1),
        };
    });
    const fieldOffsets: number[] = [];
    let fieldsHeight = 0;
    for (const fieldLayout of fieldLayouts) {
        fieldOffsets.push(fieldsHeight);
        fieldsHeight += fieldLayout.height;
    }
    const scrollRef = useRef<ScrollBoxRenderable | null>(null);
    const [scrollOffset, setScrollOffset] = useState(0);
    const [afterFieldsHeight, setAfterFieldsHeight] = useState(0);
    const titleLines = wrapFormLabel(title, width);
    const errorLines = error ? wrapFormLabel(error, width) : [];
    const actionRows = layoutFormActions(actionDefs, width);
    const menuRowBudget = preferredDropdownRows(Math.max(terminal.rows - 8, 1));
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

    const updateValue = useCallback((key: string, value: FormValue) => {
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
        if (field.kind !== "select" && field.kind !== "multiselect") {
            return;
        }
        const options = field.options ?? [];
        if (options.length === 0) {
            return;
        }

        setChoice({ kind: "open", visibleIndex });
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
        const editingList = field?.kind === "string-list" && !field.readOnly;
        if (!editingText && !editingList && onShortcut?.(input, key, event, valuesRef.current)) {
            return;
        }
        if (key.escape) {
            onClose();
            return;
        }
        if (key.pageUp || key.pageDown) {
            event.preventDefault();
            scrollRef.current?.scrollBy(key.pageUp ? -bodyHeight : bodyHeight);
            setScrollOffset(scrollRef.current?.scrollTop ?? 0);
            return;
        }
        if (editingList) {
            if (key.tab) {
                event.preventDefault();
                moveCursor(key.shift ? -1 : 1);
            }
            return;
        }
        if (key.upArrow) {
            event.preventDefault();
            moveCursor(-1);
            return;
        }
        if (key.downArrow || key.tab) {
            event.preventDefault();
            moveCursor(key.tab && key.shift ? -1 : 1);
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
            const expanded = formString(valuesRef.current, field.key) === "true";
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
                const value = formString(valuesRef.current, field.key);
                updateValue(field.key, value === "true" ? "false" : "true");
            }
        } else if ((field.kind === "select" || field.kind === "multiselect") && key.return) {
            openChoice(current);
        }
    }, { isActive: active });

    const focusedField = visibleFields[cursor];
    const choiceField = choice.kind === "open" ? visibleFields[choice.visibleIndex] : undefined;
    const choiceScalarField = choiceField?.kind === "select" || choiceField?.kind === "multiselect"
        ? choiceField
        : undefined;
    const choiceIndex = choice.kind === "open" ? choice.visibleIndex : 0;
    const choiceFieldTop = (fieldOffsets[choiceIndex] ?? 0) + (fieldLayouts[choiceIndex]?.valueTop ?? 0);
    const menuHeight = choiceScalarField
        ? Math.min(choiceScalarField.options?.length ?? 0, menuRowBudget) + FORM_MENU_BORDER_HEIGHT
        : 0;
    const preferredBodyHeight = Math.max(
        fieldsHeight + (afterFields ? afterFieldsHeight + 1 : 0),
        choiceScalarField ? choiceFieldTop + 1 + menuHeight : 1,
    );
    const compact = height < titleLines.length + errorLines.length + actionRows.length + 5;
    const bodyHeight = Math.max(1, Math.min(
        preferredBodyHeight,
        height - titleLines.length - errorLines.length - actionRows.length - (compact ? 1 : 3),
    ));

    // Keep keyboard focus in view when fields expand or the terminal resizes.
    useLayoutEffect(() => {
        if (!scrollRef.current || cursor >= visibleFields.length) {
            return;
        }
        const top = fieldOffsets[cursor] ?? 0;
        const bottom = top + (fieldLayouts[cursor]?.height ?? 1);
        const current = scrollRef.current.scrollTop;
        const next = top < current ? top : bottom > current + bodyHeight ? bottom - bodyHeight : current;
        scrollRef.current.scrollTo(Math.max(next, 0));
        setScrollOffset(scrollRef.current.scrollTop);
    }, [cursor, bodyHeight, fieldsHeight, layout.mode]);

    const dropdownPlacement = choiceScalarField
        ? bodyHeight < 3
            ? { side: "below" as const, visibleRows: bodyHeight, height: bodyHeight }
            : chooseDropdownPlacement(choiceFieldTop - scrollOffset, 1, bodyHeight, choiceScalarField.options?.length ?? 0, menuRowBudget)
        : undefined;
    const moreBelow = fieldsHeight + (afterFields ? afterFieldsHeight + 1 : 0) > scrollOffset + bodyHeight;
    const scrollHint = scrollOffset > 0 || moreBelow
        ? `${scrollOffset > 0 ? "↑" : ""}${moreBelow ? "↓" : ""} more · `
        : "";
    const hint = hintOverride ?? formHint(focusedField, width - scrollHint.length, choice.kind === "open");

    return (
        <FormShell
            titleLines={titleLines}
            errorLines={errorLines}
            preferredBodyHeight={preferredBodyHeight}
            bodyHeight={bodyHeight}
            compact={compact}
            footerHeight={actionRows.length}
            footer={actionRows.map((row, index) => (
                <ActionBar
                    key={index}
                    actions={row}
                    activeKey={cursor >= actionStart ? actionDefs[cursor - actionStart]?.key : undefined}
                    gap={3}
                    onAction={(key) => {
                        if (!active) {
                            return;
                        }
                        const actionIndex = actionDefs.findIndex((action) => action.key === key);
                        if (actionIndex >= 0) {
                            setCursor(actionStart + actionIndex);
                            runAction(actionIndex);
                        }
                    }}
                />
            ))}
            hint={`${scrollHint}${hint}${shortcutHint ? ` · ${shortcutHint}` : ""}`}
        >
            <scrollbox
                ref={scrollRef}
                width="100%"
                height={bodyHeight}
                scrollX={false}
                scrollY
                focused={false}
                viewportCulling={false}
                verticalScrollbarOptions={{ visible: false }}
                contentOptions={{ flexDirection: "column" }}
                onMouseScroll={() => {
                    setChoice({ kind: "idle" });
                    queueMicrotask(() => {
                        if (scrollRef.current && !scrollRef.current.isDestroyed) {
                            setScrollOffset(scrollRef.current.scrollTop);
                        }
                    });
                }}
            >
                {visibleFields.map((field, visibleIndex) => {
                    const selected = cursor === visibleIndex;
                    const value = values[field.key] ?? field.value;
                    const scalarValue = typeof value === "string" ? value : "";
                    const listValue = Array.isArray(value) ? value : [];
                    const options = field.kind === "string-list" ? [] : field.options ?? [];
                    const editingText = active && selected && field.kind === "text" && !field.readOnly;
                    const fieldLayout = fieldLayouts[visibleIndex]!;
                    const activate = () => {
                        setCursor(visibleIndex);
                        setChoice({ kind: "idle" });
                        if (editingText) {
                            textInputRef.current?.focus();
                        }
                    };
                    return (
                        <FormFieldRow
                            key={field.key}
                            layout={layout}
                            height={fieldLayout.height}
                            labelLines={fieldLayout.labelLines}
                            selected={selected}
                            disabled={!active || field.focusable === false}
                            disclosure={field.kind === "disclosure"
                                ? `${scalarValue === "true" ? "▾" : "▸"} ${field.label}`
                                : undefined}
                            onPress={() => {
                                activate();
                                if (field.kind === "disclosure" || (field.kind === "boolean" && !field.readOnly)) {
                                    updateValue(field.key, scalarValue === "true" ? "false" : "true");
                                }
                            }}
                        >
                            {field.kind === "string-list" ? (
                                <StringListInput
                                    value={listValue}
                                    width={layout.valueWidth}
                                    active={active && selected && !field.readOnly}
                                    onChange={(next) => updateValue(field.key, next)}
                                    onActivate={activate}
                                    onMoveOutside={(direction) => moveCursor(direction, visibleIndex)}
                                    onClose={onClose}
                                />
                            ) : editingText ? (
                                <TextInput
                                    ref={textInputRef}
                                    value={scalarValue}
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
                                            moveCursor(event.name === "tab" && event.shift ? -1 : 1, visibleIndex);
                                        } else if (event.name === "escape") {
                                            event.preventDefault();
                                            event.stopPropagation();
                                            onClose();
                                        }
                                    }}
                                />
                            ) : (
                                <Text color={selected ? "cyan" : "white"} wrap="truncate">
                                    {field.kind === "boolean"
                                        ? renderBoolean(selected, scalarValue)
                                        : field.kind === "multiselect"
                                            ? renderMultiValue(scalarValue)
                                            : renderTextValue(field, scalarValue, options, layout.valueWidth)}
                                </Text>
                            )}
                        </FormFieldRow>
                    );
                })}
                {afterFields ? (
                    <box
                        width="100%"
                        flexDirection="column"
                        flexShrink={0}
                        marginTop={1}
                        onSizeChange={function () {
                            setAfterFieldsHeight(this.height);
                        }}
                    >
                        {afterFields}
                    </box>
                ) : null}
            </scrollbox>
            {choiceScalarField && dropdownPlacement ? (
                <Box
                    position="absolute"
                    top={Math.max(0, Math.min(
                        dropdownPlacement.side === "above"
                            ? choiceFieldTop - scrollOffset - dropdownPlacement.height
                            : choiceFieldTop - scrollOffset + 1,
                        bodyHeight - dropdownPlacement.height,
                    ))}
                    left={layout.valueStart}
                    width={layout.valueWidth}
                    height={dropdownPlacement.height}
                    zIndex={10}
                >
                    <DropdownMenu
                        options={choiceScalarField.options ?? []}
                        mode={dropdownFieldMode(choiceScalarField.kind === "select" ? "select" : "multiselect")}
                        value={dropdownValueForField(
                            dropdownFieldMode(choiceScalarField.kind === "select" ? "select" : "multiselect"),
                            formString(values, choiceScalarField.key) || choiceScalarField.value,
                        )}
                        width={layout.valueWidth}
                        bordered={bodyHeight >= 3}
                        maxVisible={dropdownPlacement.visibleRows}
                        onSubmit={(next) => {
                            const submitted = Array.isArray(next) ? serializeMulti(new Set(next)) : next;
                            updateValue(choiceScalarField.key, submitted);
                            setChoice({ kind: "idle" });
                        }}
                        onClose={() => setChoice({ kind: "idle" })}
                    />
                </Box>
            ) : null}
        </FormShell>
    );
}

function formHint(field: FormField | undefined, width: number, choosing: boolean): string {
    let action = !field ? "Enter confirm"
        : field.kind === "disclosure" ? "←→ expand/collapse"
            : field.readOnly ? ""
                : field.kind === "text" ? "type to edit · Enter next"
                    : field.kind === "string-list" ? "Enter add · Backspace remove"
                        : field.kind === "boolean" ? "Space toggle"
                            : choosing && field.kind === "multiselect" ? "Space toggle · Enter confirm" : "Enter choose";
    if (width < 60) {
        action = action.replace("type to edit · ", "").replace("Backspace remove", "⌫ remove");
    }
    const parts = ["↑↓ move", action, "Esc back"].filter(Boolean);
    if (parts.join(" · ").length > width) {
        parts.shift();
    }
    return parts.join(" · ");
}

function renderTextValue(
    field: ScalarFormField,
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
        <Text color={selected ? "cyan" : "white"} bold={selected}>
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
