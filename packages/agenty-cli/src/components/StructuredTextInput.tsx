import { type InputRenderable, SyntaxStyle } from "@opentui/core";
import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";

import type { SkillDto } from "../api/types";
import {
    type ComposerDocument,
    documentFromSerialized,
    insertSkill,
    rangesForDocument,
    reconcileTextEdit,
    renderDocument,
} from "../composer/document";
import { theme } from "../consts/theme";
import { TextInput } from "./ui";

export interface StructuredTextInputHandle {
    insertSkill: (start: number, end: number, skill: Pick<SkillDto, "name" | "location" | "autoEnabled">) => void;
    setDocument: (document: ComposerDocument) => void;
    focus: () => void;
}

interface StructuredTextInputProps {
    document: ComposerDocument;
    skills: SkillDto[];
    onChange: (document: ComposerDocument) => void;
    onSubmit: (document: ComposerDocument) => void;
    onCursorChange: (offset: number) => void;
    placeholder?: string;
    focus?: boolean;
    keepFocus?: boolean;
}

export const StructuredTextInput = forwardRef<StructuredTextInputHandle, StructuredTextInputProps>(
    ({ document, skills, onChange, onSubmit, onCursorChange, placeholder, focus = true, keepFocus = false }, ref) => {
        const inputRef = useRef<InputRenderable | null>(null);
        const documentRef = useRef(document);
        documentRef.current = document;
        const [syntaxStyle] = useState(() => SyntaxStyle.fromStyles({
            skill: { fg: theme.accent, bold: true },
            "skill-warning": { fg: theme.warning, bold: true },
        }));
        const skillTypeRef = useRef<number | null>(null);

        useEffect(() => {
            const input = inputRef.current;
            if (!input) {
                return;
            }
            input.extmarks.clear();
            if (skillTypeRef.current === null) {
                skillTypeRef.current = input.extmarks.registerType("skill");
            }
            const styleId = syntaxStyle.getStyleId("skill");
            const warningStyleId = syntaxStyle.getStyleId("skill-warning");
            for (const range of rangesForDocument(document)) {
                if (range.node.type !== "skill" || styleId === null) {
                    continue;
                }
                input.extmarks.create({
                    start: range.start,
                    end: range.end,
                    virtual: true,
                    styleId: range.node.warning && warningStyleId !== null ? warningStyleId : styleId,
                    typeId: skillTypeRef.current ?? undefined,
                    data: range.node,
                });
            }
        }, [document, syntaxStyle]);

        useEffect(() => () => {
            syntaxStyle.destroy();
        }, [syntaxStyle]);

        useImperativeHandle(ref, () => ({
            insertSkill: (start, end, selectedSkill) => {
                const nextDocument = insertSkill(documentRef.current, start, end, selectedSkill);
                documentRef.current = nextDocument;
                const nextText = renderDocument(nextDocument);
                if (inputRef.current) {
                    inputRef.current.value = nextText;
                    inputRef.current.cursorOffset = start + selectedSkill.name.length + 1;
                    inputRef.current.focus();
                }
                onChange(nextDocument);
                onCursorChange(start + selectedSkill.name.length + 1);
            },
            setDocument: (nextDocument) => {
                documentRef.current = nextDocument;
                const nextText = renderDocument(nextDocument);
                if (inputRef.current) {
                    inputRef.current.value = nextText;
                    inputRef.current.cursorOffset = nextText.length;
                }
                onChange(nextDocument);
                onCursorChange(nextText.length);
            },
            focus: () => inputRef.current?.focus(),
        }), [onChange, onCursorChange]);

        const handleInput = (nextText: string) => {
            const parsedDocument = documentFromSerialized(nextText, skills);
            const nextDocument = renderDocument(parsedDocument) !== nextText
                ? parsedDocument
                : reconcileTextEdit(documentRef.current, nextText);
            documentRef.current = nextDocument;
            if (renderDocument(nextDocument) !== nextText && inputRef.current) {
                inputRef.current.value = renderDocument(nextDocument);
                inputRef.current.cursorOffset = renderDocument(nextDocument).length;
            }
            onChange(nextDocument);
            if (inputRef.current) {
                onCursorChange(inputRef.current.cursorOffset);
            }
        };

        return (
            <TextInput
                ref={inputRef}
                value={renderDocument(document)}
                onChange={handleInput}
                onSubmit={() => onSubmit(documentRef.current)}
                onCursorChange={() => {
                    if (inputRef.current) {
                        onCursorChange(inputRef.current.cursorOffset);
                    }
                }}
                syntaxStyle={syntaxStyle}
                placeholder={placeholder}
                focus={focus}
                keepFocus={keepFocus}
            />
        );
    },
);

StructuredTextInput.displayName = "StructuredTextInput";
