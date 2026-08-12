package postings_test

import (
	"testing"

	"api/internal/routes/postings"
)

func TestTheSameEmployerFoldsToOneCompanyKey(t *testing.T) {
	cases := [][2]string{
		{"Acme Yazılım A.Ş.", "Acme Yazilim AS"},
		{"Acme Yazılım A.Ş.", "ACME YAZILIM A.S."},
		{"Globex GmbH", "Globex"},
		{"Initech Ltd.", "Initech Limited"},
	}
	for _, entry := range cases {
		left := postings.CompanyKey(entry[0])
		right := postings.CompanyKey(entry[1])
		if left == "" {
			t.Fatalf("%q folded to nothing", entry[0])
		}
		if left != right {
			t.Fatalf("%q -> %q but %q -> %q", entry[0], left, entry[1], right)
		}
	}
}

func TestATitleFoldsPastMarketNoiseAndBrackets(t *testing.T) {
	left := postings.TitleKey("Senior Go Engineer")
	right := postings.TitleKey("Senior Go Engineer (m/w/d)")
	if left != right {
		t.Fatalf("%q != %q", left, right)
	}
	if postings.TitleKey("Backend Developer [Remote]") != postings.TitleKey("Backend Developer") {
		t.Fatal("a bracketed suffix changed the title key")
	}
}

func TestOneEmployerAndOneRoleShareADupeKeyAcrossSpellings(t *testing.T) {
	left := postings.DupeKey("Acme Yazılım A.Ş.", "Senior Go Engineer")
	right := postings.DupeKey("Acme Yazilim AS", "Senior Go Engineer (m/w/d)")
	if left == "" || left != right {
		t.Fatalf("dupe keys differ: %q vs %q", left, right)
	}
}

func TestAPostingWithNoCompanyOrTitleCarriesNoDupeKey(t *testing.T) {
	if key := postings.DupeKey("", "Senior Go Engineer"); key != "" {
		t.Fatalf("key = %q", key)
	}
	if key := postings.DupeKey("Acme", ""); key != "" {
		t.Fatalf("key = %q", key)
	}
}
