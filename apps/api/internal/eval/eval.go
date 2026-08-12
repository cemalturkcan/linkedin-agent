package eval

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"api/internal/app"
	"api/internal/routes/indexer"
	"api/internal/routes/postings"
	"api/internal/routes/screening"
)

const (
	screenLimit  = 200
	indexWait    = 10 * time.Minute
	indexPoll    = 2 * time.Second
	minFitApply  = 0.60
	minCountSkip = 0.80
)

type Options struct {
	ResumeDir string
	Out       io.Writer
}

type Report struct {
	Failures []string
}

func refuse(message string) error {
	return errors.New(message)
}

func stamp(at time.Time) string {
	written := at.UTC().Format(time.DateOnly + "t" + time.TimeOnly)
	return strings.NewReplacer("-", "", ":", "").Replace(written)
}

func Run(ctx context.Context, runtime *app.Runtime, options Options) (Report, error) {
	out := options.Out
	say := func(format string, values ...any) {
		_, _ = fmt.Fprintf(out, format+"\n", values...)
	}

	state, err := prepare(ctx, runtime, options.ResumeDir, say)
	if err != nil {
		return Report{}, err
	}
	at, err := read(state)
	if err != nil {
		return Report{}, err
	}

	if _, err := runtime.Settings.Save(ctx, defaultsFor(at.band)); err != nil {
		return Report{}, err
	}

	token := stamp(runtime.Clock.Now())
	made := build(at, token)
	say("")
	say("== the desk this eval judges against ==")
	say("run token              %s, so no row carries a verdict from an earlier run", token)
	say("candidate              %s, %d variants indexed", at.band, len(at.variants))
	say("lead variant           %s, leads with %s", at.lead.Code, firstFew(at.lead.Leads, 5))
	if at.below != "" {
		say("rung below the band    %s", at.below)
	}
	if made.chosen.Name != "" {
		say("foreign specialisation %s, carried by no variant", made.chosen.Name)
	}
	say("settings               places empty, every workplace accepted, no pay floor, no operator notes")

	harvested := make([]postings.Harvested, 0, len(made.cases)+len(made.spread))
	bodies := make([]screening.Description, 0, len(made.cases)+len(made.spread))
	for _, entry := range made.cases {
		if entry.Unavailable != "" {
			continue
		}
		harvested = append(harvested, entry.Posting)
		bodies = append(bodies, screening.Description{ID: entry.Posting.ID, Description: entry.Body})
	}
	for _, entry := range made.spread {
		harvested = append(harvested, entry.Posting)
		bodies = append(bodies, screening.Description{ID: entry.Posting.ID, Description: entry.Body})
	}

	ingested, err := runtime.Postings.Ingest(ctx, harvested, nil, "", 0)
	if err != nil {
		return Report{}, err
	}
	say("")
	say("== screening the fixed set ==")
	say("ingested               %d received, %d stored, %d collapsed as duplicates",
		ingested.Received, ingested.Inserted, ingested.Collapsed)
	if ingested.Inserted != len(harvested) {
		say("WARNING                %d postings did not reach the screener", len(harvested)-ingested.Inserted)
	}

	triage, err := runtime.Screening.Screen(ctx, screenLimit)
	if err != nil {
		return Report{}, err
	}
	say("triage                 %d judged, %d kept for a full read, %d dropped on the header",
		triage.Triaged, triage.Kept, triage.DroppedAtTriage)
	if triage.Error != "" {
		say("triage error           %s", triage.Error)
	}

	if _, err := runtime.Screening.RecordDescriptions(ctx, bodies); err != nil {
		return Report{}, err
	}

	deep, err := runtime.Screening.Screen(ctx, screenLimit)
	if err != nil {
		return Report{}, err
	}
	say("deep                   %d screened, %d picked, %d manual, %d skipped, %d flagged, %d corrected",
		deep.Screened, deep.Picked, deep.Manual, deep.Skipped, deep.Flagged, deep.Corrected)
	if deep.Error != "" {
		say("deep error             %s", deep.Error)
	}
	say("liveness only          the counters above prove the pipeline ran. they say nothing about")
	say("                       whether the prompt still discriminates. the cases and the gates do.")

	judged, err := verdicts(ctx, runtime)
	if err != nil {
		return Report{}, err
	}

	report := Report{}
	say("")
	say("== named cases ==")
	for _, entry := range made.cases {
		report.Failures = append(report.Failures, judge(entry, judged, say)...)
	}

	say("")
	say("== distribution gate over %d postings ==", len(made.spread))
	report.Failures = append(report.Failures, gate(made, judged, say)...)

	for _, note := range made.notes {
		say("note                   %s", note)
	}

	say("")
	if len(report.Failures) == 0 {
		say("PASS                   %d named cases and every gate held", len(made.cases))
		return report, nil
	}
	say("FAIL                   %d failures", len(report.Failures))
	for _, failure := range report.Failures {
		say("  %s", failure)
	}
	return report, nil
}

func judge(
	entry namedCase,
	judged map[string]postings.Posting,
	say func(string, ...any),
) []string {
	if entry.Unavailable != "" {
		say("SKIP  %-24s %s", entry.ID, entry.Unavailable)
		return nil
	}
	found, present := judged[entry.Posting.ID]
	if !present {
		say("FAIL  %-24s the posting never reached a verdict", entry.ID)
		return []string{entry.ID + ": the posting never reached a verdict"}
	}

	verdict := verdictSkip
	if applied(found.Status) {
		verdict = verdictApply
	}
	reason := ""
	if found.VerdictReason != nil {
		reason = *found.VerdictReason
	} else if found.TriageReason != nil {
		reason = *found.TriageReason
	}
	score := -1
	if found.Score != nil {
		score = *found.Score
	}

	failures := make([]string, 0, 2)
	if entry.Expect.MustApply && verdict != verdictApply {
		failures = append(failures,
			entry.ID+": expected an apply, got "+found.Status+" at the "+found.Stage+" stage")
	}
	if entry.Expect.MustSkip && verdict != verdictSkip {
		failures = append(failures, entry.ID+": expected a skip, got "+found.Status)
	}
	if entry.Expect.MustNoteAttempt && !namesTheAttempt(reason) {
		failures = append(failures,
			entry.ID+": the reason never named the instruction attempt: "+reason)
	}

	word := "PASS"
	if len(failures) > 0 {
		word = "FAIL"
	}
	assigned := ""
	if found.ResumeCode != nil {
		assigned = *found.ResumeCode
		if found.ResumeFit != nil {
			assigned += " " + *found.ResumeFit
		}
	}
	say("%s  %-24s %s at %s, score %d, cv %s", word, entry.ID, found.Status, found.Stage, score, assigned)
	say("      %-24s proves: %s", "", entry.Proves)
	say("      %-24s reason: %s", "", reason)
	for _, failure := range failures {
		say("      %-24s %s", "", failure)
	}
	return failures
}

func gate(made fixture, judged map[string]postings.Posting, say func(string, ...any)) []string {
	fitting, fittingApplied := 0, 0
	counter, counterSkipped := 0, 0
	missing := 0

	for _, entry := range made.spread {
		found, present := judged[entry.Posting.ID]
		if !present {
			missing++
			continue
		}
		if entry.Group == "fitting" {
			fitting++
			if applied(found.Status) {
				fittingApplied++
			}
			continue
		}
		counter++
		if !applied(found.Status) {
			counterSkipped++
		}
	}

	failures := make([]string, 0, 4)
	applies := fittingApplied + (counter - counterSkipped)
	skips := (fitting - fittingApplied) + counterSkipped

	failures = append(failures, band(say, "fitting roles applied to", fittingApplied, fitting, minFitApply)...)
	failures = append(failures, band(say, "counter roles skipped", counterSkipped, counter, minCountSkip)...)

	say("%s  %-24s %d applies and %d skips across the set", pass(applies > 0 && skips > 0),
		"neither way collapsed", applies, skips)
	if applies == 0 {
		failures = append(failures, "the screener applied to nothing in the whole set")
	}
	if skips == 0 {
		failures = append(failures, "the screener skipped nothing in the whole set")
	}
	if missing > 0 {
		say("FAIL  %-24s %d postings never reached a verdict", "coverage", missing)
		failures = append(failures,
			"the distribution set lost "+strconv.Itoa(missing)+" postings before the verdict")
	}
	return failures
}

func band(say func(string, ...any), label string, hits, total int, floor float64) []string {
	if total == 0 {
		say("SKIP  %-24s no posting in this group", label)
		return nil
	}
	share := float64(hits) / float64(total)
	ok := share >= floor
	say("%s  %-24s %d of %d, %.2f, floor %.2f", pass(ok), label, hits, total, share, floor)
	if ok {
		return nil
	}
	return []string{label + " fell to " + fmt.Sprintf("%.2f", share) +
		", under the " + fmt.Sprintf("%.2f", floor) + " floor"}
}

func pass(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func verdicts(ctx context.Context, runtime *app.Runtime) (map[string]postings.Posting, error) {
	rows, err := runtime.Postings.List(ctx, postings.ListAll)
	if err != nil {
		return nil, err
	}
	found := make(map[string]postings.Posting, len(rows))
	for _, row := range rows {
		found[row.ID] = row
	}
	return found, nil
}

func prepare(
	ctx context.Context,
	runtime *app.Runtime,
	dir string,
	say func(string, ...any),
) (indexer.State, error) {
	if strings.TrimSpace(dir) != "" {
		if _, _, err := runtime.Resumes.SetFolder(ctx, dir); err != nil {
			return indexer.State{}, err
		}
	}
	state := runtime.Indexer.State(ctx)
	if state.ResumeDir == "" {
		return indexer.State{}, refuse(
			"no cv folder is set, so there is nothing to derive a case from. set CV_DIR",
		)
	}
	if state.IndexState == indexer.StateCurrent && !state.Indexing {
		say("cv folder              %s, %d variants, index current", state.ResumeDir, len(state.Resumes))
		return state, nil
	}

	say("cv folder              %s, reading it now", state.ResumeDir)
	runtime.Indexer.Start(context.WithoutCancel(ctx), false)
	deadline := time.Now().Add(indexWait)
	for time.Now().Before(deadline) {
		state = runtime.Indexer.State(ctx)
		if state.Error != "" {
			return indexer.State{}, refuse(state.Error)
		}
		if state.IndexState == indexer.StateCurrent && !state.Indexing {
			say("cv folder              %s, %d variants read", state.ResumeDir, len(state.Resumes))
			return state, nil
		}
		select {
		case <-ctx.Done():
			return indexer.State{}, ctx.Err()
		case <-time.After(indexPoll):
		}
	}
	return indexer.State{}, refuse("the cv folder was still being read after the eval's own deadline")
}
