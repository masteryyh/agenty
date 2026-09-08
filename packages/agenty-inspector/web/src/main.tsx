import "./style.css";

import { Component, type ReactNode, StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./App";
import { Empty, ErrorMessage } from "./components";

class AppBoundary extends Component<{ children: ReactNode }, { error: string }> {
    state = { error: "" };

    static getDerivedStateFromError(error: unknown) {
        return { error: error instanceof Error ? error.message : String(error) };
    }

    render() {
        if (this.state.error) {
            return <Empty title="Unable to render this view"><ErrorMessage message={this.state.error} /><button className="quiet" onClick={() => window.location.reload()}>Reload Inspector</button></Empty>;
        }
        return this.props.children;
    }
}

const root = document.getElementById("root");
if (!root) {
    throw new Error("Missing application root");
}
createRoot(root).render(<StrictMode><AppBoundary><App /></AppBoundary></StrictMode>);
