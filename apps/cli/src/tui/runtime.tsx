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

import { createContext, useContext, type ReactNode } from "react";

export interface TuiRuntime {
	exit: (code?: number) => void;
}

const TuiRuntimeContext = createContext<TuiRuntime | null>(null);

export function TuiRuntimeProvider({
	runtime,
	children,
}: {
	runtime: TuiRuntime;
	children: ReactNode;
}) {
	return (
		<TuiRuntimeContext.Provider value={runtime}>
			{children}
		</TuiRuntimeContext.Provider>
	);
}

export function useTuiRuntime(): TuiRuntime {
	const runtime = useContext(TuiRuntimeContext);
	if (!runtime) {
		throw new Error("useTuiRuntime must be used inside TuiRuntimeProvider");
	}
	return runtime;
}
