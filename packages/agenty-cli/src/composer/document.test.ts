import { describe, expect, test } from "bun:test";

import {
    type ComposerDocument,
    documentFromSerialized,
    insertSkill,
    reconcileTextEdit,
    renderDocument,
    serializeDocument,
} from "./document";

const skill = {
    name: "awesome-masteryyh",
    location: "/Users/example/.agents/skills/awesome-masteryyh/SKILL.md",
    autoEnabled: true,
    directoryName: "awesome-masteryyh",
    description: "instructions",
    source: "agents",
};

function documentWithSkill(): ComposerDocument {
    return insertSkill({ nodes: [{ type: "text", text: "use " }] }, 4, 4, skill);
}

describe("structured composer document", () => {
    test("renders a skill as a colored display token while serializing its path", () => {
        const document = documentWithSkill();

        expect(renderDocument(document)).toBe("use $awesome-masteryyh");
        expect(serializeDocument(document)).toBe(
            "use [$awesome-masteryyh](/Users/example/.agents/skills/awesome-masteryyh/SKILL.md)",
        );
    });

    test("reconciles deleting a skill as one edit", () => {
        const document = documentWithSkill();
        const next = reconcileTextEdit(document, "use ");

        expect(next.nodes).toEqual([{ type: "text", text: "use " }]);
    });

    test("parses a pasted canonical reference into a skill node", () => {
        const document = documentFromSerialized(
            `before [$awesome-masteryyh](${skill.location}) after`,
            [skill],
        );

        expect(renderDocument(document)).toBe("before $awesome-masteryyh after");
        expect(serializeDocument(document)).toBe(
            `before [$awesome-masteryyh](${skill.location}) after`,
        );
    });

    test("keeps unknown canonical references as plain text", () => {
        const input = "[$unknown](/tmp/unknown/SKILL.md)";
        const document = documentFromSerialized(input, []);

        expect(document.nodes).toEqual([{ type: "text", text: input }]);
    });

    test("round-trips escaped closing parentheses in a skill path", () => {
        const parenthesizedSkill = { ...skill, location: "/Users/example/.agents/skills/skill)one/SKILL.md" };
        const document = insertSkill({ nodes: [] }, 0, 0, parenthesizedSkill);
        const serialized = serializeDocument(document);

        expect(serialized).toContain("skill\\)one");
        expect(documentFromSerialized(serialized, [parenthesizedSkill])).toEqual(document);
    });
});
