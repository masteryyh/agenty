import { useShallow } from "zustand/react/shallow";
import { useAppStore, type MessageStatus, type UIMessage } from "../state/store";

export interface ChatSlice {
	history: UIMessage[];
	current: UIMessage | null;
	status: MessageStatus;
	chatError: string | null;
	tokenConsumed: number;
	phrase: string | null;
	sendMessage: (text: string) => Promise<void>;
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
			abort: s.abort,
		})),
	);
}
