import { useEffect, useRef, useState } from "react";

import type {
    HubModel,
    HubModelDetails,
    ModelDownload,
    ModelDownloadPlan,
    ModelHubSource
} from "../api/types";
import { theme } from "../consts/theme";
import { useInput } from "../hooks/useInput";
import { useAppStore } from "../state/store";
import { useBottomDialogSize } from "./BottomDialog";
import { DropdownMenu } from "./DropdownMenu";
import { wrapFormLabel } from "./formLayout";
import { formatModelBytes, modelParameterLabel } from "./modelDownloadFormat";
import { ActionBar, Box, Pressable, Spinner, Text } from "./ui";

interface ModelDownloadDialogProps {
    model: HubModel;
    source: ModelHubSource;
    initialVariant?: string;
    onClose: () => void;
    onDownload: (download: ModelDownload) => void;
}

export function ModelDownloadDialog({ model, source, initialVariant, onClose, onDownload }: ModelDownloadDialogProps) {
    const client = useAppStore((state) => state.client);
    const size = useBottomDialogSize();
    const compact = size.height < 14;
    const width = Math.min(76, size.width);
    const height = Math.min(18, size.height);
    const [details, setDetails] = useState<HubModelDetails | null>(null);
    const [variant, setVariant] = useState("");
    const [plan, setPlan] = useState<ModelDownloadPlan | null>(null);
    const [checking, setChecking] = useState(true);
    const [starting, setStarting] = useState(false);
    const [choosing, setChoosing] = useState(false);
    const [focus, setFocus] = useState("cancel");
    const [revision, setRevision] = useState(0);
    const [error, setError] = useState<string | null>(null);
    const [admissionError, setAdmissionError] = useState<string | null>(null);
    const startPending = useRef(false);
    const mounted = useRef(true);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    useEffect(() => {
        let active = true;
        if (!client) {
            return;
        }
        void client.hubModel(source, model.id).then((value) => {
            if (!active) {
                return;
            }
            setDetails(value);
            const selected = value.variants.find((item) => item.id === initialVariant)
                ?? (value.variants.length === 1 ? value.variants[0] : undefined);
            setVariant(selected?.id ?? "");
            setChecking(false);
            if (!selected && value.variants.length > 1) {
                setFocus("variant");
            }
        }).catch((cause: unknown) => {
            if (active) {
                setError(cause instanceof Error ? cause.message : String(cause));
                setChecking(false);
            }
        });
        return () => {
            active = false;
        };
    }, [client, source, model.id, initialVariant]);

    useEffect(() => {
        if (!client || !variant) {
            return;
        }
        let active = true;
        setChecking(true);
        setPlan(null);
        setError(null);
        void client.prepareModelDownload({ source, repository: model.id, variant }).then((value) => {
            if (active) {
                setPlan(value);
            }
        }).catch((cause: unknown) => {
            if (active) {
                setError(cause instanceof Error ? cause.message : String(cause));
            }
        }).finally(() => {
            if (active) {
                setChecking(false);
            }
        });
        return () => {
            active = false;
        };
    }, [client, source, model.id, variant, revision]);

    // The selected variant must match the checked plan even before effect cleanup runs.
    const canDownload = !!plan && plan.variant === variant && plan.supported && plan.enoughSpace && !checking && !starting && !choosing && !admissionError;
    const multiple = (details?.variants.length ?? 0) > 1;
    const retry = !!variant && !checking && (!!error || !!admissionError || plan?.enoughSpace === false);
    const targets = [...(multiple ? ["variant"] : []), ...(retry ? ["retry"] : []), ...(canDownload ? ["download"] : []), "cancel"];
    const activate = async (key: string) => {
        if (startPending.current) {
            return;
        }
        if (key === "variant" && multiple) {
            setFocus("variant");
            setChoosing(true);
        } else if (key === "cancel") {
            onClose();
        } else if (key === "retry" && retry) {
            setAdmissionError(null);
            setRevision((value) => value + 1);
        } else if (key === "download" && canDownload && client && plan) {
            startPending.current = true;
            setStarting(true);
            setError(null);
            try {
                const download = await client.startModelDownload(plan.id, false);
                if (mounted.current) {
                    onDownload(download);
                }
            } catch (cause: unknown) {
                if (mounted.current) {
                    setAdmissionError(cause instanceof Error ? cause.message : String(cause));
                    // Admission checks capacity again; never keep a rejected plan enabled.
                    setPlan(null);
                    setRevision((value) => value + 1);
                }
            } finally {
                startPending.current = false;
                if (mounted.current) {
                    setStarting(false);
                }
            }
        }
    };

    useInput((_input, key, event) => {
        if (key.escape) {
            void activate("cancel");
        } else if (key.upArrow || key.downArrow || key.tab || key.leftArrow || key.rightArrow) {
            event.preventDefault();
            const direction = key.upArrow || key.leftArrow || key.tab && key.shift ? -1 : 1;
            const current = targets.indexOf(focus);
            setFocus(targets[Math.min(Math.max(current + direction, 0), targets.length - 1)]!);
        } else if (key.return) {
            void activate(focus);
        }
    }, { isActive: !choosing && !starting });

    const status = checking ? "Checking compatibility…" : !details ? "Cannot load model" : !details.variants.length ? "No compatible weights found"
        : !variant ? "Select a variant to check compatibility" : !plan ? "Compatibility check failed"
            : !plan.supported ? "Unsupported model" : !plan.enoughSpace ? "Not enough disk space" : "Compatible with local inference";
    const notice = error ?? admissionError ?? (!plan?.supported ? plan?.reason : !plan.enoughSpace
        ? `Need ${formatModelBytes(Math.max(plan.requiredBytes - plan.storage.availableBytes, 0))} more space.` : null);
    const noticeLines = !compact && notice ? wrapFormLabel(notice, Math.max(width - 6, 1)).slice(0, 3) : [];
    const selected = details?.variants.find((item) => item.id === variant);

    return <Box position="absolute" left={0} top={0} width={size.width} height={size.height} zIndex={120} alignItems="center" justifyContent="center">
        <Box id="model-download-dialog" width={width} height={height} flexDirection="column" borderStyle="single"
            borderColor={theme.borderStrong} backgroundColor={theme.surfaceRaised} paddingX={compact ? 1 : 2} paddingY={compact ? 0 : 1}>
            <Box height={1} flexShrink={0}><Text color={theme.accent} bold wrap="truncate">{compact ? `Download ${model.name || model.id}` : "Download model"}</Text></Box>
            <Box flexDirection="column" flexGrow={1} flexBasis={0} minHeight={0} width="100%" overflow="hidden">
                {!compact ? <Box flexDirection="column" height={2} flexShrink={0} marginTop={notice ? 0 : 1}>
                    <Text height={1} bold wrap="truncate">{model.name || model.id} · {modelParameterLabel(model)}</Text>
                    <Text height={1} dimColor wrap="truncate">{source === "huggingface" ? "Hugging Face" : "ModelScope"} / {model.id}</Text>
                </Box> : null}
                <Pressable width="100%" height={1} flexShrink={0} disabled={!multiple || starting} onPress={() => void activate("variant")}>
                    <Text dimColor>Variant  </Text>
                    <Box flexGrow={1} flexBasis={0} overflow="hidden"><Text wrap="truncate" color={focus === "variant" ? theme.selection : theme.text}>{selected?.id ?? (checking ? "Loading…" : "Select weights / quantization")}</Text></Box>
                    {multiple ? <Text color={focus === "variant" ? theme.selection : theme.textMuted}> [▾]</Text> : null}
                </Pressable>
                <Box flexDirection="column" height={1 + noticeLines.length} flexShrink={0} marginTop={compact || notice ? 0 : 1} width="100%">
                    {checking ? <Spinner label="Checking compatibility…" /> : <Text color={plan?.supported && plan.enoughSpace ? theme.success : plan || error ? theme.danger : theme.textMuted} wrap="truncate">{status}</Text>}
                    {noticeLines.length ? <Text height={noticeLines.length} color={theme.danger}>{noticeLines.join("\n")}</Text> : null}
                </Box>
                {plan && !compact ? <Box flexDirection="column" height={3} flexShrink={0} marginTop={notice ? 0 : 1} width="100%">
                    <Text height={1}>Download {formatModelBytes(plan.totalBytes)} · Free {formatModelBytes(plan.storage.availableBytes)}</Text>
                    <Text height={1} dimColor wrap="truncate">{plan.storage.volume} · {formatModelBytes(plan.storage.usedBytes)} / {formatModelBytes(plan.storage.totalBytes)} used</Text>
                    <Text height={1} dimColor wrap="truncate">Save to: {plan.directory}</Text>
                </Box> : null}
            </Box>
            <Box flexDirection="column" width="100%" height={compact ? 1 : 2} flexShrink={0} marginTop={compact ? 0 : 1}>
                <ActionBar actions={[{ key: "download", label: starting ? "Starting…" : "Download", disabled: !canDownload },
                    ...(retry ? [{ key: "retry", label: compact ? "Retry" : "Check again" }] : []), { key: "cancel", label: "Cancel", disabled: starting }]}
                activeKey={focus} onAction={(key) => void activate(key)} />
                {!compact ? <Text dimColor>↑/↓ focus · Enter select · Esc cancel</Text> : null}
            </Box>
            {choosing && details ? <Box position="absolute" left={compact ? 1 : 2} top={compact ? 1 : 5} zIndex={130}>
                <DropdownMenu options={details.variants.map((item) => ({ value: item.id, label: `${item.id} · ${formatModelBytes(item.size)}` }))}
                    mode="single" value={variant} width={Math.max(width - (compact ? 4 : 6), 1)} maxVisible={Math.max(Math.min(6, height - (compact ? 4 : 8)), 1)}
                    onClose={() => setChoosing(false)} onSubmit={(value) => {
                        if (typeof value === "string" && value !== variant) {
                            setPlan(null);
                            setAdmissionError(null);
                            setChecking(true);
                            setVariant(value);
                        }
                        setChoosing(false);
                    }} />
            </Box> : null}
        </Box>
    </Box>;
}
