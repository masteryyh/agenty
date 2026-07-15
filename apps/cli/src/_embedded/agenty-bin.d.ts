// The agenty server binary is imported with `with { type: "file" }` so Bun
// embeds it into the compiled single executable. At runtime the import
// resolves to a path string; this declaration satisfies tsc without requiring
// the binary file itself to be present during type-checking.
declare const agentyBinUrl: string;
export default agentyBinUrl;
