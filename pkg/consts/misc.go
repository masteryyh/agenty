package consts

import "github.com/google/uuid"

const (
	DefaultVectorDimension = 1536
	KnowledgeChunkSize     = 512
	KnowledgeChunkOverlap  = 64

	SearchEvaluationPrompt = `You are a search result evaluator. Analyze the search results below and determine their relevance and quality relative to the user's original query.

User's Query: %s

Search Results:
%s

Evaluate the results and respond with a JSON object (no markdown, no code fence):
{
  "quality": "<high|medium|low|no_results>",
  "relevance": <0.0-1.0>,
  "summary": "<brief summary of the most relevant findings>",
  "reasoning": "<why you rated the results this way>"
}

Guidelines:
- "high": Results directly answer the query with specific, accurate information
- "medium": Results are related but don't fully address the query
- "low": Results are tangentially related or mostly irrelevant
- "no_results": No useful results found
- relevance is a float from 0.0 (irrelevant) to 1.0 (perfect match)
- summary should be concise (1-3 sentences) highlighting the most relevant information found`
)

var (
	DefaultSystemSettingsID = uuid.MustParse("019cf9b7-a1f4-78f8-9110-15bbe177e7bc")
)
