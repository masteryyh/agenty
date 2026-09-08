import {
    Check,
    ChevronLeft,
    ChevronRight,
    Copy,
    FileSearch,
    LoaderCircle,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

export function Empty({
    title,
    children,
}: {
    title: string;
    children?: ReactNode;
}) {
    return (
        <div className="empty">
            <FileSearch size={32} strokeWidth={1.3} />
            <h2>{title}</h2>
            <div>{children}</div>
        </div>
    );
}

export function Loading() {
    return (
        <div className="loading" role="status">
            <LoaderCircle size={16} className="spin" /> Reading transcript…
        </div>
    );
}

export function ErrorMessage({ message }: { message: string }) {
    return message ? (
        <div className="error-message" role="alert">
            {message}
        </div>
    ) : null;
}

export function CopyButton({
    text,
    label = "Copy",
}: {
    text: string;
    label?: string;
}) {
    const [state, setState] = useState("");
    const timer = useRef<number | undefined>(undefined);
    useEffect(() => () => window.clearTimeout(timer.current), []);
    async function copy() {
        try {
            await navigator.clipboard.writeText(text);
            setState("Copied");
        } catch {
            setState("Copy failed");
        }
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => setState(""), 1800);
    }
    return (
        <button
            className="quiet small"
            onClick={() => void copy()}
            aria-label={label}
            title={label}
        >
            {state === "Copied" ? <Check size={13} /> : <Copy size={13} />}{" "}
            {state || label}
        </button>
    );
}

export function Pager({
    offset,
    total,
    limit,
    onChange,
}: {
    offset: number;
    total: number;
    limit: number;
    onChange: (offset: number) => void;
}) {
    return (
        <div className="pager">
            <span>
                {total === 0 ? "0" : offset + 1}–
                {Math.min(offset + limit, total)} of {total.toLocaleString()}
            </span>
            <button
                className="icon-button"
                disabled={offset === 0}
                onClick={() => onChange(Math.max(0, offset - limit))}
                aria-label="Previous page"
            >
                <ChevronLeft size={16} />
            </button>
            <button
                className="icon-button"
                disabled={offset + limit >= total}
                onClick={() => onChange(offset + limit)}
                aria-label="Next page"
            >
                <ChevronRight size={16} />
            </button>
        </div>
    );
}

export function TextView({
    text,
    language = "text",
}: {
    text: string;
    language?: string;
}) {
    const [offset, setOffset] = useState(0);
    const size = 20000;
    useEffect(() => setOffset(0), [text]);
    return (
        <div className="text-view">
            <div className="code-heading">
                <span>{language}</span>
                <CopyButton text={text} />
            </div>
            <pre>{text.slice(offset, offset + size) || "(empty)"}</pre>
            {text.length > size && (
                <Pager
                    offset={offset}
                    total={text.length}
                    limit={size}
                    onChange={setOffset}
                />
            )}
        </div>
    );
}

export function JsonView({ value }: { value: unknown }) {
    return <TextView text={JSON.stringify(value, null, 2)} language="json" />;
}

export function Badge({ value }: { value: string }) {
    return (
        <span className={"badge " + value}>{value.replaceAll("_", " ")}</span>
    );
}

export function VirtualList<T>({
    items,
    rowHeight,
    render,
    className = "",
}: {
    items: T[];
    rowHeight: number;
    render: (item: T, index: number) => ReactNode;
    className?: string;
}) {
    const ref = useRef<HTMLDivElement>(null);
    const [viewport, setViewport] = useState({ top: 0, height: 700 });
    useEffect(() => {
        const element = ref.current;
        if (!element) {
            return;
        }
        const observer = new ResizeObserver(() =>
            setViewport((value) => ({
                ...value,
                height: element.clientHeight,
            })),
        );
        observer.observe(element);
        return () => observer.disconnect();
    }, []);
    const start = Math.max(
        0,
        Math.min(items.length - 1, Math.floor(viewport.top / rowHeight) - 5),
    );
    const end = Math.min(
        items.length,
        start + Math.ceil(viewport.height / rowHeight) + 10,
    );
    return (
        <div
            ref={ref}
            className={"virtual-list " + className}
            onScroll={(event) => {
                const top = event.currentTarget.scrollTop;
                setViewport((value) => ({ ...value, top }));
            }}
        >
            <div
                style={{
                    height: items.length * rowHeight,
                    position: "relative",
                }}
            >
                {items.slice(start, end).map((item, index) => (
                    <div
                        key={start + index}
                        style={{
                            position: "absolute",
                            top: (start + index) * rowHeight,
                            height: rowHeight,
                            width: "100%",
                        }}
                    >
                        {render(item, start + index)}
                    </div>
                ))}
            </div>
        </div>
    );
}
