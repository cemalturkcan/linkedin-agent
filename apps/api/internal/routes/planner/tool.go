package planner

import (
	"slices"

	"api/internal/app/claude"
)

func queryProperties(context planContext) map[string]any {
	knobs := context.capabilities.Fetch
	properties := map[string]any{
		"label": map[string]any{
			"type": "string",
			"description": "Short lowercase slug, unique in this plan and stable across rounds " +
				"for the same query.",
		},
		"ring": map[string]any{
			"type": "string",
			"enum": knobs.Rings,
			"description": "How wide this query reaches. " + worldwide() +
				" sends no location at all; every other ring works the one place named below.",
		},
		"place": map[string]any{
			"type": "string",
			"description": "The place this query works, written the way a person would say it, " +
				"in the local spelling. Empty only when the ring is " + worldwide() + ".",
		},
		"range": map[string]any{
			"type":        "string",
			"enum":        knobs.Ranges,
			"description": "How far back this query reaches.",
		},
		"want": map[string]any{
			"type": "integer",
			"description": "How many new postings this query should bring back, 1 to " +
				itoa(context.maxWant) + ".",
		},
		"reason": map[string]any{
			"type": "string",
			"description": "One sentence naming this query's ring, why that ring earns a slot " +
				"this round, and what the screener will be asked to sort out of it.",
		},
	}
	if knobs.Keyword {
		properties["keyword"] = map[string]any{
			"type": "string",
			"description": "The search term LinkedIn itself matches this query on, written from " +
				"the stack and role words the profiles above carry. LinkedIn's matching is loose, " +
				"so it narrows the fetch without guaranteeing relevance, and nothing is filtered " +
				"here: every posting it returns still reaches the screener. An empty string sends " +
				"no term and reads the raw feed for this place and window.",
		}
	}
	if knobs.EasyApply {
		properties["easyApply"] = map[string]any{
			"type":        "boolean",
			"description": "Restrict to postings LinkedIn can auto-fill.",
		}
	}
	if knobs.RemoteOnly {
		properties["remoteOnly"] = map[string]any{
			"type":        "boolean",
			"description": "Restrict to postings marked remote.",
		}
	}
	return properties
}

func querySchema(context planContext) map[string]any {
	properties := queryProperties(context)
	required := make([]string, 0, len(properties))
	for name := range properties {
		required = append(required, name)
	}
	slices.Sort(required)
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func noteOperations() map[string]any {
	entry := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "One line of counsel for future rounds.",
			},
			"priority": map[string]any{"type": "string", "enum": NotePriorities},
		},
		"required":             []string{"text", "priority"},
		"additionalProperties": false,
	}
	revision := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "integer",
				"description": "Id of the standing note being rewritten.",
			},
			"text":     map[string]any{"type": "string"},
			"priority": map[string]any{"type": "string", "enum": NotePriorities},
		},
		"required":             []string{"id", "text", "priority"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type":        "object",
		"description": "Curate your own standing notes. Empty arrays are fine.",
		"properties": map[string]any{
			"add":    map[string]any{"type": "array", "items": entry},
			"revise": map[string]any{"type": "array", "items": revision},
			"drop": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "integer"},
				"description": "Ids of standing notes a round has disproved.",
			},
		},
		"required":             []string{"add", "revise", "drop"},
		"additionalProperties": false,
	}
}

func planTool(context planContext) claude.Tool {
	return claude.Tool{
		Name: "plan_cycle",
		Description: "Record the queries this harvest round should run, the screening budget, " +
			"and what you learned.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"queries": map[string]any{
					"type": "array",
					"description": "Between 1 and " + itoa(context.maxQueries) +
						" queries. The executor works the near rings before the far ones whatever " +
						"order you emit, so the order you choose only decides which query runs " +
						"first inside one ring.",
					"items": querySchema(context),
				},
				"screenBudget": map[string]any{
					"type": "integer",
					"description": "How many postings this round may screen, 1 to " +
						itoa(context.screenBudget) + ".",
				},
				"notes": noteOperations(),
				"rationale": map[string]any{
					"type":        "string",
					"description": "Exactly two sentences for the person watching the interface.",
				},
			},
			"required":             []string{"queries", "screenBudget", "notes", "rationale"},
			"additionalProperties": false,
		},
	}
}

func widenTool(context planContext) claude.Tool {
	return claude.Tool{
		Name: "widen_round",
		Description: "Add one broader query to a round that came up thin, and name what was " +
			"relaxed to get it.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": querySchema(context),
				"relaxed": map[string]any{
					"type":        "string",
					"enum":        Relaxations,
					"description": "The single knob this query relaxes compared to the plan.",
				},
				"reason": map[string]any{
					"type": "string",
					"description": "One sentence on why relaxing that is the cheapest way to " +
						"find new postings.",
				},
			},
			"required":             []string{"query", "relaxed", "reason"},
			"additionalProperties": false,
		},
	}
}
