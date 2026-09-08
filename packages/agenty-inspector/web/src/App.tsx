import {
    Activity,
    ArrowDownToLine,
    CircleHelp,
    Database,
    ListTree,
    MessageSquare,
    Moon,
    PanelLeft,
    RefreshCw,
    Search,
    Sun,
} from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { bytes, endpoint, formatTime, useDebounced, useResource } from "./api";
import { Empty, ErrorMessage, Loading, Pager, VirtualList } from "./components";
import type {
    Detail,
    Page,
    SessionEntry,
    SourceRef,
    System,
} from "./generated";
import {
    EventsView,
    InspectorPanel,
    IssuesView,
    MessagesView,
} from "./views";

const tabs = ["messages", "events", "issues"] as const;
type Tab = (typeof tabs)[number];
function initialNavigation() {
    const query = new URLSearchParams(window.location.search);
    const requestedTab = query.get("view");
    const tab = requestedTab === "structure" ? "messages" : requestedTab;
    return {
        session: query.get("session") ?? "",
        record: query.get("record") ?? "",
        tab: tabs.find((value) => value === tab) ?? "messages",
    };
}

export function App() {
    const [navigation, setNavigation] = useState(initialNavigation);
    const [panelOpen, setPanelOpen] = useState(() => {
        const initial = initialNavigation();
        return initial.tab === "events" && initial.record !== "";
    });
    const [sidebarOpen, setSidebarOpen] = useState(false);
    const [refresh, setRefresh] = useState(0);
    const [generation, setGeneration] = useState(0);
    const [follow, setFollow] = useState(false);
    const [connected, setConnected] = useState(false);
    const [newRecords, setNewRecords] = useState(false);
    const [query, setQuery] = useState("");
    const [agent, setAgent] = useState("");
    const [model, setModel] = useState("");
    const [issuesOnly, setIssuesOnly] = useState(false);
    const [since, setSince] = useState("");
    const [until, setUntil] = useState("");
    const [offset, setOffset] = useState(0);
    const [theme, setTheme] = useState(
        () => localStorage.getItem("inspector.theme") ?? "system",
    );
    const followRef = useRef(follow);
    followRef.current = follow;
    const search = useDebounced(query);
    const system = useResource<System>(
        endpoint("/system"),
        generation + refresh,
    );
    const sessions = useResource<Page<SessionEntry>>(
        endpoint("/sessions", {
            q: search,
            issues: issuesOnly ? "true" : undefined,
            agent,
            model,
            since,
            until,
            offset,
            limit: 100,
        }),
        generation + refresh,
    );
    const detail = useResource<Detail>(
        navigation.session ? endpoint("/sessions/" + navigation.session) : null,
        refresh,
    );
    useEffect(() => {
        document.documentElement.dataset.theme = theme;
        localStorage.setItem("inspector.theme", theme);
    }, [theme]);
    useEffect(() => {
        const events = new EventSource(endpoint("/changes"));
        let previous = "";
        events.onopen = () => setConnected(true);
        events.onerror = () => setConnected(false);
        events.onmessage = (event: MessageEvent<string>) => {
            if (event.data === previous) {
                return;
            }
            if (previous) {
                if (followRef.current) {
                    setRefresh((value) => value + 1);
                } else {
                    setNewRecords(true);
                }
            }
            previous = event.data;
            setGeneration((value) => value + 1);
        };
        return () => events.close();
    }, []);
    useEffect(() => {
        const update = () => setNavigation(initialNavigation());
        window.addEventListener("popstate", update);
        return () => window.removeEventListener("popstate", update);
    }, []);
    useEffect(() => {
        const url = new URL(window.location.href);
        for (const [key, value] of Object.entries({
            session: navigation.session,
            view: navigation.tab,
            record: navigation.record,
        })) {
            if (value) {
                url.searchParams.set(key, value);
            } else {
                url.searchParams.delete(key);
            }
        }
        window.history.replaceState(null, "", url);
    }, [navigation]);
    function selectSession(session: string) {
        setSidebarOpen(false);
        setPanelOpen(false);
        setNavigation({ session, record: "", tab: "messages" });
        setNewRecords(false);
    }
    function selectSource(source: SourceRef) {
        if (source.recordId === "") {
            return;
        }
        setPanelOpen(true);
        setFollow(false);
        setNavigation((value) => ({
            ...value,
            record: source.recordId,
            tab: "events",
        }));
    }
    function selectTab(tab: Tab) {
        setPanelOpen(false);
        setNavigation((value) => ({ ...value, tab, record: "" }));
    }
    function reload() {
        setRefresh((value) => value + 1);
        setNewRecords(false);
    }
    const agents = system.data?.agents ?? [];
    const models = system.data?.models ?? [];
    const icons = [MessageSquare, Activity, CircleHelp];
    return (
        <div className="app">
            <header className="topbar">
                <a className="brand" href="/">
                    <span className="brand-mark">
                        <ListTree size={19} />
                    </span>
                    <strong>
                        agenty<span> / inspector</span>
                    </strong>
                </a>
                <div className="data-location" title={system.data?.dataDir}>
                    <Database size={13} />
                    <span>
                        {system.data?.dataDir ?? "Reading data directory…"}
                    </span>
                    <span className="read-only">READ ONLY</span>
                </div>
                <div className="top-actions">
                    <button
                        className="icon-button mobile-sessions"
                        aria-label="Toggle sessions"
                        onClick={() => setSidebarOpen(!sidebarOpen)}
                    >
                        <PanelLeft size={16} />
                    </button>
                    <span
                        className={
                            "connection " + (connected ? "connected" : "")
                        }
                    >
                        <i />
                        {connected ? "Watching files" : "Reconnecting"}
                    </span>
                    <button
                        className="icon-button"
                        aria-label="Toggle color theme"
                        onClick={() =>
                            setTheme(theme === "dark" ? "light" : "dark")
                        }
                    >
                        {theme === "dark" ? (
                            <Sun size={16} />
                        ) : (
                            <Moon size={16} />
                        )}
                    </button>
                    <button
                        className="icon-button"
                        aria-label="Refresh data"
                        onClick={reload}
                    >
                        <RefreshCw size={16} />
                    </button>
                </div>
            </header>
            <div
                className={
                    "workspace " +
                    (navigation.session ? "has-session " : "") +
                    (sidebarOpen ? "sidebar-open" : "")
                }
            >
                <aside className="session-sidebar">
                    <div className="sidebar-title">
                        <strong>SESSIONS</strong>
                        <span>{system.data?.sessionCount ?? "—"}</span>
                    </div>
                    <div className="session-filters">
                        <label className="search-field">
                            <Search size={15} />
                            <input
                                value={query}
                                onChange={(e) => {
                                    setQuery(e.target.value);
                                    setOffset(0);
                                }}
                                placeholder="Search sessions…"
                                aria-label="Search sessions"
                            />
                        </label>
                        <div className="filter-pair">
                            <select
                                aria-label="Agent filter"
                                value={agent}
                                onChange={(e) => {
                                    setAgent(e.target.value);
                                    setOffset(0);
                                }}
                            >
                                <option value="">All agents</option>
                                {[
                                    ...new Set(
                                        [...agents, agent].filter(Boolean),
                                    ),
                                ].map((value) => (
                                    <option key={value}>{value}</option>
                                ))}
                            </select>
                            <select
                                aria-label="Model filter"
                                value={model}
                                onChange={(e) => {
                                    setModel(e.target.value);
                                    setOffset(0);
                                }}
                            >
                                <option value="">All models</option>
                                {[
                                    ...new Set(
                                        [...models, model].filter(Boolean),
                                    ),
                                ].map((value) => (
                                    <option key={value}>{value}</option>
                                ))}
                            </select>
                        </div>
                        <label className="issues-filter">
                            <input
                                type="checkbox"
                                checked={issuesOnly}
                                onChange={(event) => {
                                    setIssuesOnly(event.target.checked);
                                    setOffset(0);
                                }}
                            />{" "}
                            With issues
                        </label>
                        <details className="date-filter">
                            <summary>Filter by update date</summary>
                            <div className="filter-pair">
                                <input
                                    type="date"
                                    aria-label="Updated since"
                                    value={since}
                                    onChange={(e) => {
                                        setSince(e.target.value);
                                        setOffset(0);
                                    }}
                                />
                                <input
                                    type="date"
                                    aria-label="Updated until"
                                    value={until}
                                    onChange={(e) => {
                                        setUntil(e.target.value);
                                        setOffset(0);
                                    }}
                                />
                            </div>
                        </details>
                    </div>
                    <ErrorMessage message={sessions.error || system.error} />
                    {system.data?.error && (
                        <ErrorMessage message={system.data.error} />
                    )}
                    {sessions.loading && <Loading />}
                    {sessions.data && (
                        <>
                            <VirtualList
                                items={sessions.data.items}
                                rowHeight={94}
                                render={(entry) => (
                                    <button
                                        className={
                                            "session-item " +
                                            (entry.id === navigation.session
                                                ? "active"
                                                : "")
                                        }
                                        onClick={() => selectSession(entry.id)}
                                    >
                                        <div>
                                            <strong>
                                                {entry.title ||
                                                    "Untitled session"}
                                            </strong>
                                            {(entry.error ||
                                                entry.issueCount > 0) && (
                                                <span
                                                    className="error-dot"
                                                    title={
                                                        entry.error ||
                                                        entry.issueCount +
                                                            " issues"
                                                    }
                                                >
                                                    !
                                                </span>
                                            )}
                                        </div>
                                        <p>
                                            {entry.agent ||
                                                entry.sessionId.slice(0, 8)}
                                            <span>{bytes(entry.size)}</span>
                                        </p>
                                        <small>
                                            {formatTime(entry.updatedAt)}
                                        </small>
                                    </button>
                                )}
                            />
                            {sessions.data.total > 100 && (
                                <Pager
                                    offset={offset}
                                    total={sessions.data.total}
                                    limit={100}
                                    onChange={setOffset}
                                />
                            )}
                            {sessions.data.total === 0 && (
                                <div className="sidebar-empty">
                                    {system.data?.scanning
                                        ? "Scanning transcripts…"
                                        : "No matching sessions"}
                                </div>
                            )}
                        </>
                    )}
                    <div className="sidebar-footer">
                        <Database size={13} />
                        <span>Local JSONL transcripts</span>
                    </div>
                </aside>
                <main className="main-panel">
                    {!navigation.session && (
                        <Empty title="A closer look at your agent">
                            <p>
                                Select a session to explore its rounds,
                                messages, tool calls and structural issues.
                            </p>
                            <p className="muted">
                                Reads AGENTY_DATA_DIR, or ~/.agenty by default.
                            </p>
                        </Empty>
                    )}
                    {detail.loading && <Loading />}
                    <ErrorMessage message={detail.error} />
                    {detail.error && (
                        <button className="quiet" onClick={reload}>
                            Retry reading session
                        </button>
                    )}
                    {detail.data && (
                        <>
                            <div className="session-heading">
                                <div className="eyebrow">
                                    SESSION /{" "}
                                    {detail.data.sessionId.slice(0, 8)}
                                </div>
                                <div className="title-row">
                                    <h1>
                                        {detail.data.title ||
                                            detail.data.entry.title ||
                                            "Untitled session"}
                                    </h1>
                                    <button
                                        className={
                                            "follow-button " +
                                            (follow ? "enabled" : "")
                                        }
                                        onClick={() => {
                                            setFollow(!follow);
                                            if (!follow) {
                                                reload();
                                            }
                                        }}
                                    >
                                        <ArrowDownToLine size={13} />
                                        {follow ? "Following" : "Follow"}
                                    </button>
                                </div>
                                <div className="session-summary">
                                    <span>
                                        {detail.data.rounds.length} rounds
                                    </span>
                                    <span>
                                        {detail.data.messageCount} messages
                                    </span>
                                    <span>
                                        {detail.data.hiddenCount} hidden
                                    </span>
                                    <span>
                                        {detail.data.recordCount} records
                                    </span>
                                    <span className="model-summary">
                                        {detail.data.currentModel?.providerCode}{" "}
                                        / {detail.data.currentModel?.modelCode}
                                    </span>
                                </div>
                            </div>
                            {newRecords && (
                                <button
                                    className="new-records"
                                    onClick={reload}
                                >
                                    Files changed. Load latest snapshot{" "}
                                    <RefreshCw size={13} />
                                </button>
                            )}
                            <nav
                                className="view-tabs"
                                aria-label="Session views"
                            >
                                {tabs.map((tab, index) => {
                                    const Icon = icons[index];
                                    return (
                                        <button
                                            key={tab}
                                            className={
                                                navigation.tab === tab
                                                    ? "active"
                                                    : ""
                                            }
                                            onClick={() => selectTab(tab)}
                                        >
                                            <Icon size={15} />
                                            {tab.charAt(0).toUpperCase() +
                                                tab.slice(1)}
                                            {tab === "issues" && (
                                                <span className="tab-count">
                                                    {detail.data?.diagnostics
                                                        .length ?? 0}
                                                </span>
                                            )}
                                        </button>
                                    );
                                })}
                            </nav>
                            <div
                                className="view-content"
                                key={
                                    navigation.session +
                                    ":" +
                                    detail.data.revision
                                }
                            >
                                {navigation.tab === "messages" && (
                                    <MessagesView
                                        detail={detail.data}
                                        onSource={selectSource}
                                    />
                                )}
                                {navigation.tab === "events" && (
                                    <EventsView
                                        detail={detail.data}
                                        onSource={selectSource}
                                        selected={navigation.record}
                                    />
                                )}
                                {navigation.tab === "issues" && (
                                    <IssuesView
                                        detail={detail.data}
                                        onSource={selectSource}
                                        selected={navigation.record}
                                    />
                                )}
                            </div>
                            <footer className="snapshot-footer">
                                <span className="snapshot-dot" />
                                Snapshot {detail.data.revision.slice(0, 8)}
                                <span>
                                    Replayed through line{" "}
                                    {detail.data.replayedThrough}
                                </span>
                            </footer>
                        </>
                    )}
                </main>
                {detail.data && navigation.tab === "events" && panelOpen && (
                    <InspectorPanel
                        detail={detail.data}
                        selected={navigation.record}
                        onClose={() => {
                            setPanelOpen(false);
                            setNavigation((value) => ({
                                ...value,
                                record: "",
                            }));
                        }}
                    />
                )}
            </div>
        </div>
    );
}
