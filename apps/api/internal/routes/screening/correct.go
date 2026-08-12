package screening

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"api/internal/app/claude"
	"api/internal/app/events"
	"api/internal/routes/indexer"
)

const maxInstruction = 400

func (s *Service) correct(
	ctx context.Context,
	at desk,
	input PromptInput,
	batch []Candidate,
	judged map[string]Judged,
	tool claude.Tool,
	result *Result,
) map[string]Judged {
	if len(judged) == 0 {
		return judged
	}
	flags := s.review(ctx, input, batch, judged, result)
	if len(flags) == 0 {
		return judged
	}

	byCode := make(map[string]indexer.ResumeState, len(at.state.Resumes))
	for _, resume := range at.state.Resumes {
		byCode[strings.ToUpper(resume.Code)] = resume
	}
	system := BuildDeepPrompt(input)

	for _, posting := range batch {
		instruction, flagged := flags[posting.ID]
		if !flagged {
			continue
		}
		first := judged[posting.ID]
		result.Flagged++
		s.hub.Emit(events.TypeScreen, map[string]any{
			"phase":       "flagged",
			"id":          posting.ID,
			"instruction": instruction,
			"status":      first.Status,
		})

		second, err := s.rescreenOne(ctx, system, tool, posting, first, instruction, byCode, at, result)
		if err != nil {
			slog.Warn(
				"the correction pass could not re-screen a posting",
				"id", posting.ID,
				"err", err,
			)
			carried := first
			carried.Flagged = true
			carried.Flag = instruction
			judged[posting.ID] = carried
			continue
		}

		original := first
		second.Flagged = true
		second.Flag = instruction
		second.Rewrote = second.Status != original.Status ||
			second.Verdict.Verdict != original.Verdict.Verdict ||
			second.Reason != original.Reason
		second.Original = &original

		if err := s.commitVerdict(ctx, second, at.screenKey); err != nil {
			slog.Warn("the corrected verdict was not stored", "id", posting.ID, "err", err)
			continue
		}
		result.Corrected++
		judged[posting.ID] = second
		s.hub.Emit(events.TypeScreen, map[string]any{
			"phase":   "corrected",
			"id":      posting.ID,
			"was":     original.Status,
			"status":  second.Status,
			"changed": second.Rewrote,
			"reason":  second.Reason,
		})
	}
	return judged
}

func (s *Service) review(
	ctx context.Context,
	input PromptInput,
	batch []Candidate,
	judged map[string]Judged,
	result *Result,
) map[string]string {
	answered, err := s.call(ctx, claude.Call{
		Purpose: PurposeReview,
		System:  BuildReviewPrompt(input),
		User:    buildReviewUser(batch, judged),
		Tool:    reviewTool(),
	}, result)
	if err != nil {
		slog.Warn("the correction pass could not review the batch", "err", err)
		return nil
	}

	var decoded struct {
		Flags []struct {
			ID          string `json:"id"`
			Instruction string `json:"instruction"`
		} `json:"flags"`
	}
	if err := json.Unmarshal(answered.Input, &decoded); err != nil {
		slog.Warn("the reviewer answer could not be read", "err", err)
		return nil
	}

	flags := make(map[string]string, len(decoded.Flags))
	for _, flag := range decoded.Flags {
		if _, present := judged[flag.ID]; !present {
			continue
		}
		if _, already := flags[flag.ID]; already {
			continue
		}
		instruction := line(flag.Instruction, maxInstruction)
		if instruction == "" {
			continue
		}
		flags[flag.ID] = instruction
	}
	return flags
}

func (s *Service) rescreenOne(
	ctx context.Context,
	system string,
	tool claude.Tool,
	posting Candidate,
	previous Judged,
	instruction string,
	byCode map[string]indexer.ResumeState,
	at desk,
	result *Result,
) (Judged, error) {
	answered, err := s.call(ctx, claude.Call{
		Purpose: PurposeCorrection,
		System:  system,
		User:    buildCorrectionUser(posting, previous, instruction),
		Tool:    tool,
	}, result)
	if err != nil {
		return Judged{}, err
	}
	decisions, err := decodeDecisions(answered.Input)
	if err != nil {
		return Judged{}, err
	}
	decision, present := decisions[posting.ID]
	if !present {
		return Judged{}, refuse("the corrected answer named no posting from the batch")
	}
	verdict := shape(decision, byCode, at.rules, at.state.Resumes)
	routed := route(posting, verdict, at.rules)
	return Judged{
		ID:      posting.ID,
		Status:  routed.Status,
		Stage:   StageDeep,
		Score:   scoreOf(verdict.Score, applyFallback(verdict)),
		Reason:  routed.Reason,
		Verdict: verdict,
		TraceID: answered.TraceID,
	}, nil
}
