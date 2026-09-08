import {
    ArrowRight,
    ChevronDown,
    GitBranch,
    MessageSquare,
    Search,
} from "lucide-react";
import { useEffect, useState } from "react";

import { endpoint, formatTime, useDebounced, useResource } from "./api";
import {
    Badge,
    CopyButton,
    Empty,
    ErrorMessage,
    Loading,
    Pager,
    TextView,
    VirtualList,
} from "./components";
import { Content } from "./content";
import type {
    Detail,
    MessageNode,
    Page,
    Record as EventRecord,
    RecordDetail,
    RoundNode,
    SourceRef,
} from "./generated";

export interface ViewProps {
    detail: Detail;
    onSource: (source: SourceRef) => void;
    selected: string;
}

export function MessagesView({ detail, onSource }: Pick<ViewProps, "detail" | "onSource">) {
    return (
        <div className="messages-scroll">
            <div className="session-root">
                <GitBranch size={18} />
                <strong>Session</strong>
                <code>{detail.sessionId.slice(0, 8)}</code>
                <CopyButton text={detail.sessionId} label="Copy ID" />
            </div>
            <div className="session-facts">
                <span>
                    REASONING <strong>{detail.reasoningEffort || "off"}</strong>
                </span>
                <span>
                    CONTEXT WINDOW{" "}
                    <strong>{detail.contextWindow.toLocaleString()}</strong>
                </span>
            </div>
            <p className="muted path-line">
                {detail.cwd || "No working directory recorded"}
            </p>
            {!detail.complete && (
                <ErrorMessage
                    message={
                        "Replay stopped after line " +
                        detail.replayedThrough +
                        ". Later records remain available in Events."
                    }
                />
            )}
            {detail.rounds.map((round, index) => (
                <Round
                    key={round.source.recordId}
                    round={round}
                    detail={detail}
                    initialOpen={index === detail.rounds.length - 1}
                    onSource={onSource}
                />
            ))}
            {detail.rounds.length === 0 && (
                <Empty title="No replayed rounds">
                    Open Events to inspect the original records.
                </Empty>
            )}
        </div>
    );
}

function Round({
    round,
    detail,
    initialOpen,
    onSource,
}: Pick<ViewProps, "detail" | "onSource"> & {
    round: RoundNode;
    initialOpen: boolean;
}) {
    const [open, setOpen] = useState(initialOpen);
    const [offset, setOffset] = useState(0);
    const result = useResource<Page<MessageNode>>(
        open
            ? endpoint(
                "/sessions/" +
                      detail.entry.id +
                      "/rounds/" +
                      round.id +
                      "/messages",
                { revision: detail.revision, offset, limit: 30 },
            )
            : null,
    );
    return (
        <section className="round">
            <button
                className="round-heading"
                onClick={() => setOpen(!open)}
                aria-expanded={open}
            >
                <ChevronDown
                    size={16}
                    className={open ? "" : "closed-chevron"}
                />
                <strong>Round {round.sequence}</strong>
                <Badge value={round.status} />
                <span className="round-model">{round.model.modelCode}</span>
                <span className="muted">{round.messageCount} messages</span>
            </button>
            {open && (
                <div className="round-content">
                    <div className="round-info">
                        <span>
                            {formatTime(round.startedAt)} →{" "}
                            {round.endedAt
                                ? formatTime(round.endedAt)
                                : "No terminal event recorded"}
                        </span>
                    </div>
                    <div className="usage-strip">
                        Recorded round usage{" "}
                        <strong>{round.usage.total.toLocaleString()}</strong>{" "}
                        tokens{" "}
                        <span>
                            Input {round.usage.input.toLocaleString()} · Output{" "}
                            {round.usage.output.toLocaleString()}
                        </span>
                    </div>
                    {round.error && <ErrorMessage message={round.error} />}
                    {result.loading && <Loading />}
                    <ErrorMessage message={result.error} />
                    {result.data?.items.map((node) => (
                        <MessageCard
                            key={node.source.recordId}
                            node={node}
                            detail={detail}
                            onSource={onSource}
                        />
                    ))}
                    {result.data && result.data.total > 30 && (
                        <Pager
                            offset={offset}
                            total={result.data.total}
                            limit={30}
                            onChange={setOffset}
                        />
                    )}
                </div>
            )}
        </section>
    );
}

type MessageKind = "User" | "Assistant" | "Tool";

function messageKind(message: MessageNode["message"]): MessageKind {
    if (
        message.content.some(
            (block) =>
                block.type === "tool_result" ||
                block.type === "shell_call_output",
        )
    ) {
        return "Tool";
    }
    return message.role === "assistant" ? "Assistant" : "User";
}

function MessageCard({
    node,
    detail,
    onSource,
}: Pick<ViewProps, "detail" | "onSource"> & { node: MessageNode }) {
    const message = node.message;
    const hidden = message.visibility === "hidden";
    const kind = messageKind(message);
    return (
        <article
            className={
                "message message-" +
                kind.toLowerCase() +
                " " +
                (hidden ? "hidden-message" : "")
            }
        >
            <div className="message-heading">
                <div className="message-role">
                    <MessageSquare size={14} />
                    <strong>{kind}</strong>
                </div>
                {hidden && <Badge value="hidden" />}
                <div className="message-reference">
                    <code>{message.id.slice(-8)}</code>
                    <span className="line-tag">L{node.source.line}</span>
                </div>
            </div>
            <div className="message-body">
                <div className="message-meta">
                    <span>
                        {formatTime(message.createdAt)} ·{" "}
                        {message.content.length} blocks
                        {message.usage
                            ? " · " +
                              message.usage.total.toLocaleString() +
                              " tokens"
                            : ""}
                    </span>
                </div>
                {hidden ? (
                    <Content
                        blocks={message.content}
                        roundId={message.roundId}
                        tools={detail.tools}
                        onSource={onSource}
                        codeText
                        codeLanguage="hidden message"
                    />
                ) : (
                    <Content
                        blocks={message.content}
                        roundId={message.roundId}
                        tools={detail.tools}
                        onSource={onSource}
                    />
                )}
            </div>
        </article>
    );
}

const eventTypeLabels: { [key: string]: string } = {
    session_started: "Session created",
    session_created: "Session created",
    session_model_set: "Model changed",
    session_reasoning_effort_set: "Reasoning changed",
    session_cwd_set: "Working directory changed",
    round_started: "Round started",
    message_appended: "Message appended",
    session_compacted: "Session compacted",
    session_metadata_refreshed: "Session metadata refreshed",
    round_ended: "Round ended",
    session_title_set: "Title changed",
    invalid: "Invalid record",
    empty: "Empty line",
};

function eventLabel(type: string): string {
    const label = eventTypeLabels[type];
    if (label) {
        return label;
    }
    return type
        .split("_")
        .filter(Boolean)
        .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
        .join(" ");
}

const eventTypes = [
    "session_started",
    "session_model_set",
    "session_reasoning_effort_set",
    "session_cwd_set",
    "round_started",
    "message_appended",
    "session_compacted",
    "session_metadata_refreshed",
    "round_ended",
    "session_title_set",
    "invalid",
    "empty",
];

export function EventsView({ detail, onSource, selected }: ViewProps) {
    const [query, setQuery] = useState("");
    const [type, setType] = useState("");
    const [round, setRound] = useState("");
    const [offset, setOffset] = useState(0);
    const search = useDebounced(query);
    const result = useResource<Page<EventRecord>>(
        endpoint("/sessions/" + detail.entry.id + "/events", {
            revision: detail.revision,
            q: search,
            type,
            roundId: round,
            offset,
            limit: 100,
        }),
    );
    return (
        <div className="events-view">
            <div className="view-toolbar">
                <label className="search-field">
                    <Search size={15} />
                    <input
                        value={query}
                        onChange={(e) => {
                            setQuery(e.target.value);
                            setOffset(0);
                        }}
                        placeholder="Search raw records…"
                        aria-label="Search raw records"
                    />
                </label>
                <select
                    value={type}
                    onChange={(e) => {
                        setType(e.target.value);
                        setOffset(0);
                    }}
                    aria-label="Event type"
                >
                    <option value="">All events</option>
                    {eventTypes.map((value) => (
                        <option key={value} value={value}>
                            {eventLabel(value)}
                        </option>
                    ))}
                </select>
                <select
                    value={round}
                    onChange={(e) => {
                        setRound(e.target.value);
                        setOffset(0);
                    }}
                    aria-label="Filter round"
                >
                    <option value="">All rounds</option>
                    {detail.rounds.map((value) => (
                        <option key={value.source.recordId} value={value.id}>
                            Round {value.sequence}
                        </option>
                    ))}
                </select>
            </div>
            <div className="event-table-head">
                <span>SEQ</span>
                <span>EVENT</span>
                <span>CONTENT</span>
            </div>
            {result.loading && <Loading />}
            <ErrorMessage message={result.error} />
            {result.data && (
                <>
                    <VirtualList
                        items={result.data.items}
                        rowHeight={62}
                        render={(record) => (
                            <button
                                className={
                                    "event-row " +
                                    (record.id === selected ? "active" : "")
                                }
                                onClick={() =>
                                    onSource({
                                        recordId: record.id,
                                        line: record.line,
                                        seq: record.seq,
                                    })
                                }
                            >
                                <span className="event-seq">
                                    <strong>
                                        {record.seq > 0 ? "#" + record.seq : "—"}
                                    </strong>
                                </span>
                                <span className="event-type">
                                    <span
                                        className={
                                            "event-dot " +
                                            (record.error
                                                ? "error"
                                                : record.type)
                                        }
                                    />
                                    {eventLabel(record.type)}
                                    <small>{formatTime(record.wroteAt)}</small>
                                </span>
                                <span className="event-summary">
                                    {record.error ||
                                        record.summary ||
                                        "Empty line"}
                                    {!record.applied && (
                                        <small>Not applied to replay</small>
                                    )}
                                </span>
                            </button>
                        )}
                    />
                    {result.data.total === 0 && (
                        <Empty title="No matching records">
                            Try another event type or search term.
                        </Empty>
                    )}
                    <Pager
                        offset={offset}
                        total={result.data.total}
                        limit={100}
                        onChange={setOffset}
                    />
                </>
            )}
        </div>
    );
}

export function IssuesView({ detail, onSource }: ViewProps) {
    return (
        <div className="issues-view">
            <p className="muted">
                Diagnostics preserve the original file order. Missing terminal
                records do not prove that a process is still running.
            </p>
            {detail.diagnostics.length === 0 && (
                <Empty title="No structural issues found">
                    All records decoded and replayed successfully.
                </Empty>
            )}
            {detail.diagnostics.map((issue, index) => (
                <button
                    className="issue"
                    key={index}
                    onClick={() => onSource(issue.source)}
                    disabled={!issue.source.recordId}
                >
                    <Badge value={issue.severity} />
                    <div>
                        <strong>{issue.code.replaceAll("_", " ")}</strong>
                        <p>{issue.message}</p>
                    </div>
                    <span className="line-tag">
                        {issue.source.line ? "L" + issue.source.line : ""}
                    </span>
                    <ArrowRight size={14} />
                </button>
            ))}
            {detail.tools.length > 0 && (
                <>
                    <div className="section-label">TOOL RELATIONS</div>
                    {detail.tools.map((tool, index) => (
                        <div className="tool-relation" key={index}>
                            <strong>{tool.name || "Unknown tool"}</strong>
                            <Badge value={tool.status} />
                            <code>{tool.id}</code>
                            <div className="related">
                                {[...tool.calls, ...tool.results].map(
                                    (source, i) => (
                                        <button
                                            key={i}
                                            className="quiet small"
                                            onClick={() => onSource(source)}
                                        >
                                            {i < tool.calls.length
                                                ? "Call"
                                                : "Result"}{" "}
                                            L{source.line}
                                        </button>
                                    ),
                                )}
                            </div>
                        </div>
                    ))}
                </>
            )}
        </div>
    );
}

function prettyRawJSON(raw: string): string {
    try {
        const value: unknown = JSON.parse(raw);
        return JSON.stringify(value, null, 2) ?? raw;
    } catch {
        return raw;
    }
}

type MetadataField = {
    key: string;
    label: string;
    value: string;
};

const metadataLabels: { [key: string]: string } = {
    cwd: "Working directory",
    model: "Model",
    provider: "Provider",
    timezone: "Timezone",
    "reasoning-effort": "Reasoning effort",
};

function isObject(value: unknown): value is { [key: string]: unknown } {
    return typeof value === "object" && value !== null && !Array.isArray(value);
}

function metadataFields(data: RecordDetail): MetadataField[] | null {
    const payload = data.envelope?.payload;
    if (!isObject(payload) || !isObject(payload.message)) {
        return null;
    }
    const content = payload.message.content;
    if (!Array.isArray(content) || content.length !== 1 || !isObject(content[0])) {
        return null;
    }
    if (content[0].type !== "text" || typeof content[0].text !== "string") {
        return null;
    }

    const text = content[0].text.trim();
    if (!text.startsWith("<metadata")) {
        return null;
    }
    const document = new DOMParser().parseFromString(text, "application/xml");
    if (
        document.documentElement?.nodeName !== "metadata" ||
        document.getElementsByTagName("parsererror").length > 0
    ) {
        return null;
    }
    return Array.from(document.documentElement.children).map((element) => ({
        key: element.tagName,
        label: metadataLabels[element.tagName] ?? eventLabel(element.tagName),
        value: element.textContent?.trim() ?? "",
    }));
}

type EventField = {
    label: string;
    value: string;
};

function displayValue(value: unknown): string | null {
    if (typeof value === "string") {
        return value || null;
    }
    if (typeof value === "number") {
        return Number.isFinite(value) ? value.toLocaleString() : String(value);
    }
    if (typeof value === "boolean") {
        return value ? "Yes" : "No";
    }
    return null;
}

function modelValue(value: unknown): string | null {
    if (!isObject(value)) {
        return null;
    }
    const provider = displayValue(value.providerCode);
    const model = displayValue(value.modelCode);
    return [provider, model].filter(Boolean).join(" / ") || null;
}

function eventFields(data: RecordDetail): EventField[] {
    const payload = data.envelope?.payload;
    if (!isObject(payload)) {
        return [];
    }
    const fields: EventField[] = [];
    const add = (label: string, value: unknown) => {
        const text = displayValue(value);
        if (text !== null) {
            fields.push({ label, value: text });
        }
    };
    const addModel = (value: unknown) => add("Model", modelValue(value));
    const addUsage = (value: unknown) => {
        if (!isObject(value)) {
            return;
        }
        add("Input tokens", value.input);
        add("Output tokens", value.output);
        add("Total tokens", value.total);
    };
    const addMessage = (value: unknown) => {
        if (!isObject(value)) {
            return;
        }
        add("Message ID", value.id);
        add("Role", value.role);
        add("Visibility", value.visibility || "visible");
        add("Content blocks", Array.isArray(value.content) ? value.content.length : null);
        addModel(value.model);
        addUsage(value.usage);
    };

    switch (data.record.type) {
        case "session_started":
            add("Session ID", payload.sessionId);
            addModel(payload.model);
            add("Context window", payload.contextWindow);
            add("Reasoning effort", payload.reasoningEffort);
            add("Working directory", payload.cwd);
            break;
        case "session_model_set":
            addModel(payload.model);
            add("Context window", payload.contextWindow);
            break;
        case "session_reasoning_effort_set":
            add("Reasoning effort", payload.reasoningEffort);
            break;
        case "session_cwd_set":
            add("Working directory", payload.cwd ?? "Cleared");
            break;
        case "round_started":
            add("Round ID", payload.roundId);
            add("Round sequence", payload.sequence);
            addModel(payload.model);
            add("Context window", payload.contextWindow);
            add("Reasoning effort", payload.reasoningEffort);
            add("Working directory", payload.cwd);
            break;
        case "message_appended":
        case "session_metadata_refreshed":
            addMessage(payload.message);
            break;
        case "session_compacted":
            add("Compaction ID", payload.compactionId);
            add("Trigger", payload.trigger);
            add("Context tokens before", payload.contextTokensBefore);
            addUsage(payload.usage);
            break;
        case "round_ended":
            add("Status", payload.status);
            addUsage(payload.usage);
            add("Error", payload.error);
            break;
        case "session_title_set":
            add("Title", payload.title);
            break;
    }
    return fields;
}

type MessageSummaryItem = {
    label: string;
    text?: string;
};

type MessageSummary = {
    title: string;
    items: MessageSummaryItem[];
};

function previewMessageText(value: unknown, limit = 280): string | undefined {
    if (typeof value !== "string") {
        return undefined;
    }
    const text = value.trim();
    if (!text) {
        return undefined;
    }
    return text.length > limit ? text.slice(0, limit) + "…" : text;
}

function structuredResultText(value: unknown): string | undefined {
    if (typeof value !== "string") {
        return undefined;
    }
    try {
        const parsed: unknown = JSON.parse(value);
        if (isObject(parsed) && typeof parsed.content === "string") {
            return parsed.content;
        }
    } catch {
        // Keep ordinary text results as-is.
    }
    return undefined;
}

function previewToolInput(value: unknown, limit = 320): string | undefined {
    if (value === undefined) {
        return undefined;
    }
    if (typeof value === "string") {
        return previewMessageText(value, limit);
    }
    try {
        const encoded = JSON.stringify(value, null, 2);
        return previewMessageText(encoded, limit);
    } catch {
        return previewMessageText(String(value), limit);
    }
}

function messageBlockSummary(
    block: unknown,
    nested = false,
): MessageSummaryItem | null {
    if (!isObject(block) || typeof block.type !== "string") {
        return null;
    }
    switch (block.type) {
        case "text":
            return {
                label: nested ? "Result content" : "Text",
                text:
                    (nested && structuredResultText(block.text)) ||
                    previewMessageText(block.text),
            };
        case "reasoning":
            return {
                label: "Thinking",
                text:
                    previewMessageText(block.reasoning) ??
                    (block.redacted || block.signature
                        ? "Thinking content is encrypted."
                        : "No thinking content recorded."),
            };
        case "tool_use":
            return {
                label: "Tool call" +
                    (typeof block.name === "string" ? " · " + block.name : ""),
                text: previewToolInput(block.input),
            };
        case "tool_result": {
            const content = Array.isArray(block.content)
                ? block.content
                    .map((item) => messageBlockSummary(item, true))
                    .filter((item): item is MessageSummaryItem => item !== null)
                : [];
            return {
                label: "Tool result",
                text: content
                    .map((item) => item.text)
                    .filter((item): item is string => Boolean(item))
                    .join("\n"),
            };
        }
        case "shell_call":
            return {
                label: "Shell command",
                text: Array.isArray(block.commands)
                    ? previewMessageText(
                        block.commands.filter(
                            (command): command is string =>
                                typeof command === "string",
                        ).join("\n"),
                    )
                    : undefined,
            };
        case "shell_call_output": {
            const outputs = Array.isArray(block.output) ? block.output : [];
            const text = outputs
                .filter(isObject)
                .flatMap((output) => [output.stdout, output.stderr])
                .filter((item): item is string => typeof item === "string")
                .join("\n");
            return { label: "Shell output", text: previewMessageText(text) };
        }
        case "apply_patch_call": {
            const operation = isObject(block.operation)
                ? [block.operation.type, block.operation.path]
                    .filter((item): item is string => typeof item === "string")
                    .join(" · ")
                : "";
            return {
                label: operation ? "Patch · " + operation : "Patch",
                text: previewMessageText(block.patch),
            };
        }
        case "image":
            return {
                label: "Image",
                text: previewMessageText(block.mimeType),
            };
        default:
            return { label: block.type.replaceAll("_", " ") };
    }
}

function messageContentSummary(data: RecordDetail): MessageSummary | null {
    const payload = data.envelope?.payload;
    if (!isObject(payload) || !isObject(payload.message)) {
        return null;
    }
    const content = payload.message.content;
    if (!Array.isArray(content)) {
        return null;
    }
    const items = content
        .map((block) => messageBlockSummary(block))
        .filter((item): item is MessageSummaryItem => item !== null);
    const isToolOutput = content.some(
        (block) =>
            isObject(block) &&
            (block.type === "tool_result" || block.type === "shell_call_output"),
    );
    const role = displayValue(payload.message.role);
    const title = isToolOutput
        ? "Tool output"
        : role === "assistant"
            ? "Assistant message"
            : role === "user"
                ? "User message"
                : role
                    ? role.charAt(0).toUpperCase() + role.slice(1) + " message"
                    : "Message";
    return { title, items };
}

function EventFields({ fields }: { fields: EventField[] }) {
    if (fields.length === 0) {
        return null;
    }
    return (
        <>
            <div className="section-label">EVENT DETAILS</div>
            <dl className="event-fields-grid">
                {fields.map((field) => (
                    <div key={field.label}>
                        <dt>{field.label}</dt>
                        <dd>{field.value}</dd>
                    </div>
                ))}
            </dl>
        </>
    );
}

function MetadataInfo({ fields }: { fields: MetadataField[] }) {
    return (
        <div className="event-metadata">
            <div className="section-label">SESSION METADATA</div>
            {fields.length === 0 ? (
                <p className="muted">No metadata fields recorded.</p>
            ) : (
                <dl className="metadata-grid">
                    {fields.map((field) => (
                        <div key={field.key}>
                            <dt>{field.label}</dt>
                            <dd>{field.value || "—"}</dd>
                        </div>
                    ))}
                </dl>
            )}
        </div>
    );
}

function MessageSummaryInfo({ summary }: { summary: MessageSummary }) {
    return (
        <div className="event-message-summary">
            <div className="section-label">MESSAGE CONTENT</div>
            <div className="message-summary-card">
                <div className="message-summary-heading">
                    <strong>{summary.title}</strong>
                    <span>{summary.items.length} blocks</span>
                </div>
                {summary.items.length === 0 ? (
                    <p className="muted">No readable content recorded.</p>
                ) : (
                    <div className="message-summary-items">
                        {summary.items.map((item, index) => (
                            <div className="message-summary-item" key={index}>
                                <strong>{item.label}</strong>
                                {item.text && <p>{item.text}</p>}
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}

function EventInfo({ data }: { data: RecordDetail }) {
    const { record } = data;
    const details = eventFields(data);
    const metadata = metadataFields(data);
    const message = messageContentSummary(data);
    return (
        <>
            <div className="event-facts">
                <div>
                    <span>TYPE</span>
                    <strong>{eventLabel(record.type)}</strong>
                </div>
                <div>
                    <span>SEQ</span>
                    <strong>{record.seq > 0 ? "#" + record.seq : "—"}</strong>
                </div>
                <div>
                    <span>TIME</span>
                    <strong>{formatTime(record.wroteAt)}</strong>
                </div>
            </div>
            {record.roundId && (
                <div className="event-context">
                    <span>ROUND</span>
                    <code>{record.roundId}</code>
                </div>
            )}
            <EventFields fields={details} />
            {metadata ? (
                <MetadataInfo fields={metadata} />
            ) : message ? (
                <MessageSummaryInfo summary={message} />
            ) : record.summary ? (
                <div className="event-summary-card">
                    <span>SUMMARY</span>
                    <p>{record.summary}</p>
                </div>
            ) : null}
        </>
    );
}

export function InspectorPanel({
    detail,
    selected,
    onClose,
}: {
    detail: Detail;
    selected: string;
    onClose: () => void;
}) {
    const [tab, setTab] = useState("info");
    useEffect(() => setTab("info"), [selected]);
    const result = useResource<RecordDetail>(
        selected
            ? endpoint(
                "/sessions/" + detail.entry.id + "/records/" + selected,
                {
                    revision: detail.revision,
                },
            )
            : null,
    );
    return (
        <aside
            className={"inspector-panel " + (selected ? "has-selection" : "")}
        >
            <div className="panel-heading">
                <strong>INSPECTOR</strong>
                <button className="quiet small" onClick={onClose}>
                    Close
                </button>
            </div>
            {!selected && (
                <Empty title="Inspect an event">
                    Select an event in Events or Issues to see its structured
                    details and exact source.
                </Empty>
            )}
            {result.loading && <Loading />}
            <ErrorMessage message={result.error} />
            {result.data && (
                <>
                    <div className="selection-info">
                        <h2>
                            {eventLabel(result.data.record.type)}
                            <span>
                                {result.data.record.seq > 0
                                    ? " · #" + result.data.record.seq
                                    : ""}
                            </span>
                        </h2>
                        <p className="muted">
                            Offset {result.data.record.offset.toLocaleString()} ·{" "}
                            {result.data.record.length.toLocaleString()} bytes
                        </p>
                    </div>
                    <div className="detail-tabs">
                        <button
                            className={tab === "info" ? "active" : ""}
                            onClick={() => setTab("info")}
                        >
                            Info
                        </button>
                        <button
                            className={tab === "raw" ? "active" : ""}
                            onClick={() => setTab("raw")}
                        >
                            Raw
                        </button>
                    </div>
                    <div className="detail-body">
                        {result.data.record.error && (
                            <ErrorMessage message={result.data.record.error} />
                        )}
                        {tab === "raw" ? (
                            <>
                                <TextView
                                    text={prettyRawJSON(result.data.raw)}
                                    language="json"
                                />
                                {result.data.rawBase64 && (
                                    <>
                                        <ErrorMessage message="Invalid UTF-8 is replaced in the text view. Base64 below preserves the original bytes." />
                                        <TextView
                                            text={result.data.rawBase64}
                                            language="original bytes · base64"
                                        />
                                    </>
                                )}
                            </>
                        ) : (
                            <EventInfo data={result.data} />
                        )}
                        <p className="muted footnote">
                            wroteAt is populated from the event time by core. It
                            is not an independent disk-write timestamp.
                        </p>
                    </div>
                </>
            )}
        </aside>
    );
}
