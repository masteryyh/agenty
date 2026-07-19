import type { ParsedArgs } from "./utils";

export function handleVersion(_: ParsedArgs): void {
    process.stdout.write(`${process.env.AGENTY_CLI_VERSION ?? "dev"}\n`);
}
