import {
    ArrowUpRight,
    Brain,
    Code2,
    Image as ImageIcon,
    Terminal,
    Wrench,
} from "lucide-react";
import { useState } from "react";
import Markdown from "react-markdown";

import { Badge, JsonView, TextView } from "./components";
import type { ContentBlock, JsonValue, SourceRef, ToolRelation } from "./generated";

interface Props {
    blocks: ContentBlock[];
    roundId?: string;
    tools?: ToolRelation[];
    onSource?: (source: SourceRef) => void;
    codeText?: boolean;
    codeLanguage?: string;
}

export function Content({
    blocks,
    roundId,
    tools = [],
    onSource,
    codeText = false,
    codeLanguage,
}: Props) {
    return (
        <div className="content-blocks">
            {blocks.map((block, index) => (
                <Block
                    key={index}
                    block={block}
                    roundId={roundId}
                    tools={tools}
                    onSource={onSource}
                    codeText={codeText}
                    codeLanguage={codeLanguage}
                />
            ))}
        </div>
    );
}

type StructuredToolResult = { [key: string]: JsonValue } & {
    content: string;
};

function isJsonObject(value: unknown): value is { [key: string]: JsonValue } {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function parseContentResult(blocks: ContentBlock[]): StructuredToolResult | null {
    if (blocks.length !== 1 || blocks[0].type !== "text") {
        return null;
    }

    try {
        const value: unknown = JSON.parse(blocks[0].text);
        if (!isJsonObject(value) || typeof value.content !== "string") {
            return null;
        }
        return value as StructuredToolResult;
    } catch {
        return null;
    }
}

function ToolResultContent({
    blocks,
    roundId,
    tools,
    onSource,
    relation,
}: Props & { relation?: ToolRelation }) {
    const structured = parseContentResult(blocks);
    if (!structured) {
        return (
            <Content
                blocks={blocks}
                roundId={roundId}
                tools={tools}
                onSource={onSource}
                codeText
                codeLanguage="tool result"
            />
        );
    }

    const metadata: { [key: string]: JsonValue } = {};
    for (const [key, value] of Object.entries(structured)) {
        if (key !== "content") {
            metadata[key] = value;
        }
    }

    return (
        <div className="structured-tool-result">
            {Object.keys(metadata).length > 0 && <JsonView value={metadata} />}
            <TextView
                text={structured.content}
                language={relation?.name ? relation.name + " content" : "content"}
            />
        </div>
    );
}

function Block({
    block,
    roundId,
    tools,
    onSource,
    codeText,
    codeLanguage,
}: Omit<Props, "blocks"> & { block: ContentBlock }) {
    const [raw, setRaw] = useState(false);
    const [previewImage, setPreviewImage] = useState(false);
    const callId =
        "callId" in block
            ? block.callId
            : block.type === "tool_use"
                ? block.id
                : block.type === "tool_result"
                    ? block.toolUseId
                    : "";
    const relation = tools?.find(
        (tool) => tool.id === callId && tool.roundId === roundId,
    );
    const isResult =
        block.type === "tool_result" || block.type === "shell_call_output";
    const links = relation
        ? isResult
            ? relation.calls
            : relation.results
        : [];
    const icon =
        block.type === "reasoning" ? (
            <Brain size={14} />
        ) : block.type.includes("shell") ? (
            <Terminal size={14} />
        ) : block.type === "image" ? (
            <ImageIcon size={14} />
        ) : (
            <Wrench size={14} />
        );
    if (block.type === "reasoning") {
        const hasPlaintext = block.reasoning.trim().length > 0;
        const encrypted = !hasPlaintext && (block.redacted || !!block.signature);
        return (
            <div className="reasoning-block">
                <div className="reasoning-heading">
                    {icon}
                    <span>Thinking</span>
                </div>
                {hasPlaintext && (
                    <div className="reasoning-text">{block.reasoning}</div>
                )}
                {!hasPlaintext && encrypted && (
                    <p className="reasoning-notice">
                        Thinking content is encrypted.
                    </p>
                )}
                {!hasPlaintext && !encrypted && (
                    <p className="reasoning-notice">
                        No thinking content recorded.
                    </p>
                )}
            </div>
        );
    }
    if (block.type === "text") {
        return (
            <div className="text-block">
                <div className="block-label">
                    <span>TEXT</span>
                    {!codeText && (
                        <button
                            className="quiet small"
                            onClick={() => setRaw(!raw)}
                        >
                            <Code2 size={12} />
                            {raw ? "Rendered" : "Raw text"}
                        </button>
                    )}
                </div>
                {codeText ? (
                    <TextView
                        text={block.text}
                        language={codeLanguage ?? "text"}
                    />
                ) : raw || block.text.length > 20000 ? (
                    <TextView text={block.text} />
                ) : (
                    <div className="markdown">
                        <Markdown
                            skipHtml
                            components={{
                                img: ({ alt, src }) => (
                                    <span className="external-image">
                                        [External image: {alt ?? src}]
                                    </span>
                                ),
                                a: ({ href, children }) => (
                                    <a
                                        href={href}
                                        target="_blank"
                                        rel="noreferrer noopener"
                                    >
                                        {children}
                                    </a>
                                ),
                            }}
                        >
                            {block.text}
                        </Markdown>
                    </div>
                )}
            </div>
        );
    }
    return (
        <div
            className={"block " + (isResult ? "result-block" : "")}
        >
            <div className="block-heading">
                {icon}
                <span>
                    {block.type === "tool_use"
                        ? block.name
                        : block.type.replaceAll("_", " ")}
                </span>
                {relation && relation.status !== "matched" && (
                    <Badge value={relation.status} />
                )}
                {block.type === "tool_result" && block.isError && (
                    <Badge value="error" />
                )}
                {callId && <code className="call-id">{callId}</code>}
            </div>
            <div className="block-body">
                {block.type === "tool_use" && <JsonView value={block.input} />}
                {block.type === "tool_result" && (
                    <ToolResultContent
                        blocks={block.content}
                        roundId={roundId}
                        tools={tools}
                        onSource={onSource}
                        relation={relation}
                    />
                )}
                {block.type === "shell_call" && (
                    <>
                        <TextView
                            text={(block.commands ?? []).join("\n")}
                            language="shell"
                        />
                        <p className="muted">
                            Timeout: {block.timeoutMs ?? "default"} ms · Output
                            limit: {block.maxOutputLength ?? "default"}
                        </p>
                    </>
                )}
                {block.type === "shell_call_output" && (
                    <>
                        {(block.output ?? []).map((output, index) => (
                            <div key={index}>
                                <div className="output-label">
                                    Command {index + 1}{" "}
                                    <Badge value={output.outcome.type} />{" "}
                                    <span>
                                        Exit {output.outcome.exitCode ?? "—"}
                                    </span>
                                </div>
                                <TextView
                                    text={output.stdout}
                                    language="stdout"
                                />
                                {output.stderr && (
                                    <TextView
                                        text={output.stderr}
                                        language="stderr"
                                    />
                                )}
                            </div>
                        ))}
                    </>
                )}
                {block.type === "apply_patch_call" && (
                    <>
                        <p>
                            <Badge value={block.source} />{" "}
                            {block.operation?.type}{" "}
                            <code>{block.operation?.path}</code>
                        </p>
                        {block.operation?.moveTo && (
                            <p>Move to {block.operation.moveTo}</p>
                        )}
                        <TextView
                            text={block.patch ?? block.operation?.diff ?? ""}
                            language="diff"
                        />
                    </>
                )}
                {block.type === "image" && (
                    <>
                        <p>
                            {block.mimeType} ·{" "}
                            {block.data
                                ? Math.round(
                                    block.data.length * 0.75,
                                ).toLocaleString() + " bytes"
                                : "URI reference"}
                        </p>
                        {block.uri && (
                            <TextView text={block.uri} language="source URI" />
                        )}
                        {block.data &&
                            /^image\/(png|jpeg|gif|webp)$/.test(
                                block.mimeType,
                            ) && (
                            <>
                                <button
                                    className="quiet"
                                    onClick={() =>
                                        setPreviewImage(!previewImage)
                                    }
                                >
                                    {previewImage
                                        ? "Hide image"
                                        : "Preview embedded image"}
                                </button>
                                {previewImage && (
                                    <img
                                        className="embedded-image"
                                        src={
                                            "data:" +
                                                block.mimeType +
                                                ";base64," +
                                                block.data
                                        }
                                        alt="Session attachment"
                                    />
                                )}
                            </>
                        )}
                    </>
                )}
                {links.length > 0 && (
                    <div className="related">
                        {links.map((link, index) => (
                            <button
                                key={index}
                                className="quiet small"
                                onClick={() => onSource?.(link)}
                            >
                                <ArrowUpRight size={12} />
                                {isResult ? "Call" : "Result"} · line{" "}
                                {link.line}
                                {relation?.status === "ambiguous"
                                    ? " (candidate)"
                                    : ""}
                            </button>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}
