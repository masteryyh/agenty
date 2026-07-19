export const streamingPhrases = [
	"Brainstorming...",
	"Thundering...",
	"Processing...",
	"Connecting dots...",
	"Exploring ideas...",
	"Crafting a response...",
	"Pondering...",
	"Working on it...",
	"Discombobulating...",
	"Firing synapses...",
	"Cooking up something good...",
	"Deciphering...",
	"Seasoning...",
	"Precolating...",
	"Flibbertigibbeting...",
];

let cursor = 0;
export function pickStreamingPhrase(): string {
	const idx = cursor % streamingPhrases.length;
	cursor += 1;
	return streamingPhrases[idx];
}
