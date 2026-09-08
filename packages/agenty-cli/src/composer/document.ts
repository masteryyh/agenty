import type { SkillDto } from "../api/types";

export type ComposerNode =
    | { type: "text"; text: string }
    | { type: "skill"; name: string; path: string; warning?: boolean };

export interface ComposerDocument {
    nodes: ComposerNode[];
}

export interface ComposerRange {
    node: ComposerNode;
    start: number;
    end: number;
}

export function emptyDocument(): ComposerDocument {
    return { nodes: [] };
}

export function renderDocument(document: ComposerDocument): string {
    return document.nodes.map((node) => node.type === "text" ? node.text : `$${node.name}`).join("");
}

function escapeMarkdownPath(path: string): string {
    return path.replaceAll("\\", "\\\\").replaceAll(")", "\\)");
}

export function serializeDocument(document: ComposerDocument): string {
    return document.nodes.map((node) => {
        if (node.type === "text") {
            return node.text;
        }
        return `[$${node.name}](${escapeMarkdownPath(node.path)})`;
    }).join("");
}

export function documentFromText(text: string): ComposerDocument {
    return { nodes: text ? [{ type: "text", text }] : [] };
}

export function documentFromSerialized(text: string, skills: SkillDto[]): ComposerDocument {
    const nodes: ComposerNode[] = [];
    let textStart = 0;
    let offset = 0;
    while (offset < text.length) {
        const start = text.indexOf("[$", offset);
        if (start < 0) {
            break;
        }
        const nameEnd = text.indexOf("](", start + 2);
        if (nameEnd < 0) {
            break;
        }
        const close = findEscapedCloseParen(text, nameEnd + 2);
        if (close < 0) {
            break;
        }
        const name = text.slice(start + 2, nameEnd);
        const path = unescapeMarkdownPath(text.slice(nameEnd + 2, close));
        const skill = skills.find((candidate) => candidate.name === name && candidate.location === path);
        if (!skill) {
            offset = close + 1;
            continue;
        }
        appendNode(nodes, { type: "text", text: text.slice(textStart, start) });
        appendNode(nodes, { type: "skill", name: skill.name, path: skill.location, warning: !skill.autoEnabled });
        offset = close + 1;
        textStart = offset;
    }
    appendNode(nodes, { type: "text", text: text.slice(textStart) });
    return { nodes };
}

function findEscapedCloseParen(text: string, start: number): number {
    let escaped = false;
    for (let index = start; index < text.length; index += 1) {
        const character = text[index];
        if (character === ")" && !escaped) {
            return index;
        }
        if (character === "\\" && !escaped) {
            escaped = true;
        } else {
            escaped = false;
        }
    }
    return -1;
}

function unescapeMarkdownPath(path: string): string {
    let result = "";
    let escaped = false;
    for (const character of path) {
        if (escaped) {
            result += character === ")" || character === "\\" ? character : `\\${character}`;
            escaped = false;
        } else if (character === "\\") {
            escaped = true;
        } else {
            result += character;
        }
    }
    return escaped ? `${result}\\` : result;
}

export function rangesForDocument(document: ComposerDocument): ComposerRange[] {
    const ranges: ComposerRange[] = [];
    let offset = 0;
    for (const node of document.nodes) {
        const length = node.type === "text" ? node.text.length : node.name.length + 1;
        ranges.push({ node, start: offset, end: offset + length });
        offset += length;
    }
    return ranges;
}

function appendNode(nodes: ComposerNode[], node: ComposerNode): void {
    if (node.type === "text" && node.text === "") {
        return;
    }
    const previous = nodes[nodes.length - 1];
    if (previous?.type === "text" && node.type === "text") {
        nodes[nodes.length - 1] = { type: "text", text: previous.text + node.text };
        return;
    }
    nodes.push(node);
}

function sliceDocument(document: ComposerDocument, start: number, end: number): ComposerNode[] {
    const result: ComposerNode[] = [];
    for (const range of rangesForDocument(document)) {
        if (range.node.type === "skill") {
            if (range.start >= start && range.end <= end) {
                appendNode(result, range.node);
            }
            continue;
        }
        const overlapStart = Math.max(start, range.start);
        const overlapEnd = Math.min(end, range.end);
        if (overlapStart < overlapEnd) {
            appendNode(result, {
                type: "text",
                text: range.node.text.slice(overlapStart - range.start, overlapEnd - range.start),
            });
        }
    }
    return result;
}

function documentLength(document: ComposerDocument): number {
    return renderDocument(document).length;
}

export function replaceRange(
    document: ComposerDocument,
    start: number,
    end: number,
    replacement: ComposerNode[],
): ComposerDocument {
    const length = documentLength(document);
    const safeStart = Math.max(0, Math.min(start, length));
    const safeEnd = Math.max(safeStart, Math.min(end, length));
    const nodes: ComposerNode[] = [];
    for (const node of sliceDocument(document, 0, safeStart)) {
        appendNode(nodes, node);
    }
    for (const node of replacement) {
        appendNode(nodes, node);
    }
    for (const node of sliceDocument(document, safeEnd, length)) {
        appendNode(nodes, node);
    }
    return { nodes };
}

function commonPrefixLength(left: string, right: string): number {
    let index = 0;
    while (index < left.length && index < right.length && left[index] === right[index]) {
        index += 1;
    }
    return index;
}

function commonSuffixLength(left: string, right: string, prefix: number): number {
    let length = 0;
    while (
        length < left.length - prefix &&
        length < right.length - prefix &&
        left[left.length - length - 1] === right[right.length - length - 1]
    ) {
        length += 1;
    }
    return length;
}

export function reconcileTextEdit(document: ComposerDocument, nextText: string): ComposerDocument {
    const previousText = renderDocument(document);
    if (previousText === nextText) {
        return document;
    }
    const prefix = commonPrefixLength(previousText, nextText);
    const suffix = commonSuffixLength(previousText, nextText, prefix);
    const oldEnd = previousText.length - suffix;
    const newEnd = nextText.length - suffix;
    return replaceRange(document, prefix, oldEnd, documentFromText(nextText.slice(prefix, newEnd)).nodes);
}

export function insertSkill(
    document: ComposerDocument,
    start: number,
    end: number,
    skill: Pick<SkillDto, "name" | "location" | "autoEnabled">,
): ComposerDocument {
    return replaceRange(document, start, end, [{
        type: "skill",
        name: skill.name,
        path: skill.location,
        warning: !skill.autoEnabled,
    }]);
}
