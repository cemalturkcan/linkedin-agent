package screening

import (
	"api/internal/app/claude"
	"api/internal/routes/settings"
)

func triageTool() claude.Tool {
	return claude.Tool{
		Name: "triage_jobs",
		Description: "Record one triage decision for every job id in the batch. " +
			"Exactly one entry per input id, no extra ids.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decisions": map[string]any{
					"type":        "array",
					"description": "One entry per job id supplied in the user message.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "The job id exactly as given in the input.",
							},
							"keep": map[string]any{
								"type": "boolean",
								"description": "True when the posting is worth fetching and " +
									"reading in full.",
							},
							"promise": map[string]any{
								"type": "integer",
								"description": "0-100, how much the header suggests reading will " +
									"pay off.",
							},
							"reason": map[string]any{
								"type":        "string",
								"description": "One short English sentence justifying the decision.",
							},
						},
						"required":             []string{"id", "keep", "promise", "reason"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"decisions"},
			"additionalProperties": false,
		},
	}
}

func statedPaySchema() map[string]any {
	return map[string]any{
		"type":        "object",
		"description": "Compensation the posting itself names. present=false when it names none.",
		"properties": map[string]any{
			"present": map[string]any{"type": "boolean"},
			"amount": map[string]any{
				"type":        "integer",
				"description": "Low end of the stated range, whole number, 0 when absent.",
			},
			"currency": map[string]any{
				"type":        "string",
				"description": "Three-letter code, empty when absent.",
			},
			"period": map[string]any{
				"type":        "string",
				"description": "hour, day, month or year; empty when absent.",
			},
		},
		"required":             []string{"present", "amount", "currency", "period"},
		"additionalProperties": false,
	}
}

func tailoredSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"description": "Whether a purpose-written resume would materially change this " +
			"application.",
		"properties": map[string]any{
			"needed": map[string]any{"type": "boolean"},
			"focus": map[string]any{
				"type": "string",
				"description": "One sentence naming what a tailored resume should lead with; " +
					"empty when not needed.",
			},
		},
		"required":             []string{"needed", "focus"},
		"additionalProperties": false,
	}
}

func decisionSchema(available, languages []string) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":        "string",
				"description": "The job id exactly as given in the input.",
			},
			"verdict": map[string]any{
				"type": "string",
				"enum": []string{VerdictApply, VerdictSkip},
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "One short English sentence justifying the verdict.",
			},
			"score": map[string]any{
				"type":        "integer",
				"description": "Match strength from 0 to 100.",
			},
			"seniority": map[string]any{
				"type":        "string",
				"enum":        settings.SeniorityValues,
				"description": "Level the posting is pitched at.",
			},
			"workplace": map[string]any{
				"type":        "string",
				"enum":        Workplaces,
				"description": "Workplace type the posting describes.",
			},
			"contractType": map[string]any{
				"type":        "string",
				"enum":        ContractTypes,
				"description": "Basis the posting hires on.",
			},
			"postingLang": map[string]any{
				"type":        "string",
				"description": "Two-letter code of the language the posting is written in.",
			},
			"agency": map[string]any{
				"type": "boolean",
				"description": "True when the hiring party is a recruiting firm or staffing " +
					"agency, not the employer.",
			},
			"statedPay": statedPaySchema(),
			"resumeCode": map[string]any{
				"type":        "string",
				"enum":        available,
				"description": "Resume variant whose real content fits the posting's core work.",
			},
			"resumeLang": map[string]any{"type": "string", "enum": languages},
			"resumeFit": map[string]any{
				"type":        "string",
				"enum":        Fits,
				"description": "How well the chosen variant already covers this posting.",
			},
			"tailoredResume": tailoredSchema(),
		},
		"required": []string{
			"id", "verdict", "reason", "score", "seniority", "workplace", "contractType",
			"postingLang", "agency", "statedPay", "resumeCode", "resumeLang", "resumeFit",
			"tailoredResume",
		},
		"additionalProperties": false,
	}
}

func deepTool(available, languages []string) claude.Tool {
	return claude.Tool{
		Name: "screen_jobs",
		Description: "Record one final verdict for every job id in the batch. " +
			"Exactly one entry per input id, no extra ids.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"decisions": map[string]any{
					"type":        "array",
					"description": "One entry per job id supplied in the user message.",
					"items":       decisionSchema(available, languages),
				},
			},
			"required":             []string{"decisions"},
			"additionalProperties": false,
		},
	}
}

func reviewTool() claude.Tool {
	return claude.Tool{
		Name: "flag_verdicts",
		Description: "Flag the verdicts in this batch that deserve a second look. " +
			"An empty list is a correct answer.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"flags": map[string]any{
					"type": "array",
					"description": "At most one entry per job id, and only for a verdict that is " +
						"concretely wrong.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id": map[string]any{
								"type":        "string",
								"description": "The job id exactly as given in the input.",
							},
							"instruction": map[string]any{
								"type": "string",
								"description": "One sentence naming what the screener should look " +
									"at again. Never dictate the verdict.",
							},
						},
						"required":             []string{"id", "instruction"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"flags"},
			"additionalProperties": false,
		},
	}
}

func lessonTool() claude.Tool {
	return claude.Tool{
		Name: "write_lessons",
		Description: "Record the durable lessons drawn from these outcomes, and retire lessons " +
			"the outcomes contradict.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"lessons": map[string]any{
					"type": "array",
					"description": "At most " + itoa(MaxLessons) + " lessons. An empty array is a " +
						"correct answer when the outcomes support nothing.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"text": map[string]any{
								"type": "string",
								"description": "One sentence, under 40 words, actionable where it " +
									"lands.",
							},
							"evidence": map[string]any{
								"type":        "string",
								"description": "Short count naming the rows behind it.",
							},
							"scope": map[string]any{
								"type": "string",
								"enum": Scopes,
								"description": "planning when the lesson changes which postings to " +
									"go looking for, screening when it changes how a posting " +
									"already in hand is judged.",
							},
						},
						"required":             []string{"text", "evidence", "scope"},
						"additionalProperties": false,
					},
				},
				"retire": map[string]any{
					"type":        "array",
					"description": "Ids of existing lessons the new outcomes contradict. Empty is normal.",
					"items":       map[string]any{"type": "integer"},
				},
			},
			"required":             []string{"lessons", "retire"},
			"additionalProperties": false,
		},
	}
}
