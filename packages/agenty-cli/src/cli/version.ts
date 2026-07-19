import type { ParsedArgs } from "./utils";
import { AGENTY_VERSION } from "../version";

export function handleVersion(_: ParsedArgs): void {
	process.stdout.write(`${AGENTY_VERSION}\n`);
}
