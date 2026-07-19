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
import type { CliOptions } from "../config";
import type { AgentDto, ChatSessionDto, ModelDto } from "../api/types";
import {
	useAppStore,
	type OverlayKind,
	type ToastMsg,
} from "../state/store";

export interface AppSlice {
	phase: "loading" | "error" | "wizard" | "ready";
	initError: string | null;
	opts: CliOptions;
	agent: AgentDto | null;
	model: ModelDto | null;
	session: ChatSessionDto | null;
	runtimeVersion: string;
	overlay: OverlayKind;
	toast: ToastMsg | null;
	thinkingEnabled: boolean;
	thinkingLevel: string;
	init: () => Promise<void>;
	reset: () => void;
	newSession: () => Promise<void>;
	switchModel: (model: ModelDto) => Promise<void>;
	resumeSession: (session: ChatSessionDto) => Promise<void>;
	switchAgent: (agent: AgentDto) => Promise<void>;
	setOverlay: (overlay: OverlayKind) => void;
	setToast: (text: string, error?: boolean) => void;
	notify: (text: string, error?: boolean) => void;
	setThinking: (enabled: boolean, level: string) => void;
	compactSession: () => Promise<void>;
	setCwd: (path: string | null) => Promise<void>;
}

export function useApp(): AppSlice {
	return useAppStore(
		useShallow((s) => ({
			phase: s.phase,
			initError: s.initError,
			opts: s.opts,
			agent: s.agent,
			model: s.model,
			session: s.session,
			runtimeVersion: s.runtimeVersion,
			overlay: s.overlay,
			toast: s.toast,
			thinkingEnabled: s.thinkingEnabled,
			thinkingLevel: s.thinkingLevel,
			init: s.init,
			reset: s.reset,
			newSession: s.newSession,
			switchModel: s.switchModel,
			resumeSession: s.resumeSession,
			switchAgent: s.switchAgent,
			setOverlay: s.setOverlay,
			setToast: s.setToast,
			notify: s.notify,
			setThinking: s.setThinking,
			compactSession: s.compactSession,
			setCwd: s.setCwd,
		})),
	);
}
