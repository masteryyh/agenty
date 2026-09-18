import { useShallow } from "zustand/react/shallow";

import type { ChatSessionDto, ModelDto, PermissionMode } from "../api/types";
import type { CliOptions } from "../config";
import {
    type OverlayKind,
    type ToastMsg,
    useAppStore,
} from "../state/store";

export interface AppSlice {
    phase: "loading" | "error" | "wizard" | "ready";
    initError: string | null;
    opts: CliOptions;
    model: ModelDto | null;
    session: ChatSessionDto | null;
    overlay: OverlayKind;
    toast: ToastMsg | null;
    thinkingEnabled: boolean;
    thinkingLevel: string;
    init: () => Promise<void>;
    reset: () => void;
    newSession: () => Promise<void>;
    switchModel: (model: ModelDto) => Promise<void>;
    resumeSession: (session: ChatSessionDto) => Promise<void>;
    setOverlay: (overlay: OverlayKind) => void;
    setToast: (text: string, error?: boolean) => void;
    notify: (text: string, error?: boolean) => void;
    setThinking: (enabled: boolean, level: string) => void;
    setCwd: (path: string | null) => Promise<void>;
    setPermissionMode: (mode: PermissionMode) => Promise<void>;
    togglePermissionMode: () => Promise<void>;
}

export function useApp(): AppSlice {
    return useAppStore(
        useShallow((s) => ({
            phase: s.phase,
            initError: s.initError,
            opts: s.opts,
            model: s.model,
            session: s.session,
            overlay: s.overlay,
            toast: s.toast,
            thinkingEnabled: s.thinkingEnabled,
            thinkingLevel: s.thinkingLevel,
            init: s.init,
            reset: s.reset,
            newSession: s.newSession,
            switchModel: s.switchModel,
            resumeSession: s.resumeSession,
            setOverlay: s.setOverlay,
            setToast: s.setToast,
            notify: s.notify,
            setThinking: s.setThinking,
            setCwd: s.setCwd,
            setPermissionMode: s.setPermissionMode,
            togglePermissionMode: s.togglePermissionMode,
        })),
    );
}
