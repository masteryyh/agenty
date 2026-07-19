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
