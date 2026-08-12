package postings

import "testing"

func TestTheListLeavesTheDescriptionBehind(t *testing.T) {
	full := Posting{
		ID:               "1",
		Title:            "Senior Go Engineer",
		Description:      "a body of several thousand characters that no list ever draws",
		DescriptionState: "ok",
	}

	listed := withoutBody(full)
	if listed.Description != "" {
		t.Fatalf("the list carries %d characters of body", len(listed.Description))
	}
	if listed.DescriptionState != "ok" || listed.Title != full.Title {
		t.Fatalf("the row lost more than the body: %+v", listed)
	}
	if full.Description == "" {
		t.Fatal("the caller's own posting was emptied")
	}
}
