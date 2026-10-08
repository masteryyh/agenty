// Printable ASCII only: the same mark works in the TUI and plain terminal output.
export const AGENTY_PARTICLE_LINES = [
    ".''. .''' :''' :. : '::' :  : '.",
    ":..: : .. :''  : ':  ::   ::    :",
    ":  : '..: :... :  :  ::   ::  .'",
] as const;

export const AGENTY_PARTICLE_WIDTH = Math.max(...AGENTY_PARTICLE_LINES.map((line) => line.length));
export const AGENTY_TAGLINE = "Agent platform with full offline capability.";
