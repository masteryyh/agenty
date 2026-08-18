import { useShallow } from "zustand/react/shallow";

import { type MessageStatus, type UIMessage, useAppStore } from "../state/store";

export interface ChatSlice {
    history: UIMessage[];
    current: UIMessage | null;
    status: MessageStatus;
    chatError: string | null;
    tokenConsumed: number;
    phrase: string | null;
    sendMessage: (text: string) => Promise<void>;
    compactSession: () => Promise<void>;
    abort: () => void;
}

export function useChat(): ChatSlice {
    return useAppStore(
        useShallow((s) => ({
            history: s.history,
            current: s.current,
            status: s.status,
            chatError: s.chatError,
            tokenConsumed: s.tokenConsumed,
            phrase: s.phrase,
            sendMessage: s.sendMessage,
            compactSession: s.compactSession,
            abort: s.abort,
        })),
    );
}
