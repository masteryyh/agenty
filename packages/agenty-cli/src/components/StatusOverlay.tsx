import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { KeyValueList } from "./List";
import { Panel } from "./Panel";

export function StatusOverlay() {
    const dialogSize = useBottomDialogSize();
    const agent = useAppStore((s) => s.agent);
    const model = useAppStore((s) => s.model);
    const session = useAppStore((s) => s.session);
    const thinkingEnabled = useAppStore((s) => s.thinkingEnabled);
    const thinkingLevel = useAppStore((s) => s.thinkingLevel);
    const history = useAppStore((s) => s.history);
    const tokenConsumed = useAppStore((s) => s.tokenConsumed);
    const setOverlay = useAppStore((s) => s.setOverlay);

    useInput((_input, key) => {
        if (key.escape) {
            setOverlay(null);
        }
    });

    const thinking = thinkingEnabled
        ? `on${thinkingLevel ? ` (${thinkingLevel} effort)` : ""}`
        : "off";
    const rows = [
        { key: "Session", value: session?.id ?? "?" },
        { key: "Agent", value: agent?.name ?? "?" },
        { key: "Model", value: `${model?.providerName ?? "?"} · ${model?.name ?? "?"}` },
        { key: "Thinking", value: thinking },
        { key: "Messages", value: String(history.length) },
        { key: "Context", value: `${session?.contextWindow ?? 0}/${tokenConsumed}` },
        { key: "CWD", value: session?.cwd ?? process.cwd() },
    ];
    return (
        <Panel title="Status" hint="Esc to close">
            <KeyValueList rows={rows} availableWidth={dialogSize.width} />
        </Panel>
    );
}
