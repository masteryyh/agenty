import type { StoreClient } from "./store";

const unexpected = async (): Promise<never> => {
    throw new Error("unexpected core client call in store test");
};

const defaults: StoreClient = {
    compactSession: unexpected,
    close: () => {},
    completeInitialization: unexpected,
    createModel: unexpected,
    createMcpServer: unexpected,
    createProvider: unexpected,
    createSession: unexpected,
    deleteModel: unexpected,
    deleteProvider: unexpected,
    eventCursor: () => undefined,
    getModel: unexpected,
    getSession: unexpected,
    isInitialized: async () => false,
    listMcpServerLogs: unexpected,
    listMcpServers: unexpected,
    listModels: unexpected,
    listProviders: unexpected,
    listProviderModels: unexpected,
    listSessions: unexpected,
    listSessionSummaries: async () => [],
    listSkills: async () => ({ skills: [], diagnostics: [] }),
    loginMcpServer: unexpected,
    onMcpEvent: () => () => {},
    onClose: () => () => {},
    onCompactionEvent: () => () => {},
    onSessionEvent: () => () => {},
    onStreamFrame: () => () => {},
    prepareSession: unexpected,
    removeMcpServer: unexpected,
    resolveToolApproval: async () => {},
    resolveModelInput: unexpected,
    setMcpEnabled: unexpected,
    setSessionCwd: unexpected,
    setSessionModel: unexpected,
    setSessionPermissionMode: unexpected,
    setSessionReasoningEffort: unexpected,
    setToolDialect: unexpected,
    startSession: unexpected,
    stopSession: async () => {},
    subscribeMcp: () => {},
    subscribeSession: () => {},
    subscribeSessionAndWait: async () => {},
    updateModel: unexpected,
    updateMcpServer: unexpected,
    updateProvider: unexpected,
};

export function createStoreClient(overrides: Partial<StoreClient> = {}): StoreClient {
    return { ...defaults, ...overrides };
}
