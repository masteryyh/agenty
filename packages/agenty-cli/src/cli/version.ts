import { AGENTY_VERSION } from "../version";
import type { ParsedArgs } from "./utils";

export function handleVersion(_: ParsedArgs): void {
    process.stdout.write(`${AGENTY_VERSION}\n`);
}
