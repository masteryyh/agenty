/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
