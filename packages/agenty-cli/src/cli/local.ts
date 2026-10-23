import { readFile } from "node:fs/promises";
import { resolve } from "node:path";
import { setTimeout } from "node:timers/promises";

import type { AgentyClient } from "../api/client";
import type { LocalInferenceConfig } from "../api/types";
import { action, CliError, flag, type ParsedArgs, render, requireFlag } from "./utils";

export async function handleLocal(client: AgentyClient, args: ParsedArgs): Promise<void> {
    const command = args.positionals[1];
    if (command === "status") {
        const state = await client.localInferenceRuntime();
        render(args, state, () => process.stdout.write(`${JSON.stringify(state, null, 2)}\n`));
        return;
    }
    if (command === "enable" || command === "disable") {
        const result = await client.configureLocalInference({ enabled: command === "enable" });
        action(args, result, `Local inference ${command}d.`);
        return;
    }
    if (command === "configure") {
        const data: unknown = JSON.parse(await readFile(requireFlag(args, "file"), "utf8"));
        if (typeof data !== "object" || data === null || !("enabled" in data) || !("workers" in data)) {
            throw new CliError("local config requires enabled and workers fields");
        }
        // Core validates the complete typed configuration before persisting it.
        const result = await client.configureLocalInference(data as LocalInferenceConfig);
        action(args, result, "Local inference configuration saved.");
        return;
    }
    if (command === "install") {
        const url = flag(args, "url");
        const file = flag(args, "file");
        let result;
        if (file) {
            if (url || flag(args, "backend")) {
                throw new CliError("--file cannot be combined with --url or --backend");
            }
            result = await client.installWorkerFile(resolve(file), requireFlag(args, "sha256"));
            while (result.status === "installing" || result.status === "downloading") {
                await setTimeout(100);
                result = await client.workerInstallation(result.id);
            }
            if (result.status !== "installed") {
                throw new CliError(result.error || `Worker installation ${result.status}`);
            }
            action(args, result, `Worker installed: ${result.worker?.id ?? result.id}`);
            return;
        } else if (url) {
            result = await client.installWorker(url, requireFlag(args, "sha256"));
        } else {
            const backend = requireFlag(args, "backend");
            const bundle = (await client.workerBundles()).find((item) => item.backend === backend);
            if (!bundle) {
                throw new CliError("No released worker package matches this platform/backend.");
            }
            result = await client.installWorker(bundle.url, bundle.sha256);
        }
        action(args, result, `Worker installation started: ${result.id}`);
        return;
    }
    if (command === "installations") {
        const result = await client.workerInstallations();
        render(args, result, () => process.stdout.write(`${JSON.stringify(result, null, 2)}\n`));
        return;
    }
    if (command === "cancel-installation") {
        const id = args.positionals[2];
        if (!id) {
            throw new CliError("usage: local cancel-installation <id>");
        }
        await client.cancelWorkerInstallation(id);
        action(args, { id }, "Worker installation cancelled.");
        return;
    }
    throw new CliError("usage: local <status|enable|disable|configure|install|installations|cancel-installation>");
}
