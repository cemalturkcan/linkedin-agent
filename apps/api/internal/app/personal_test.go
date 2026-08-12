package app

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestNoCheckoutOfThisRepositoryNamesThePersonWhoWroteIt(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("root: %v", err)
	}

	named := regexp.MustCompile(`(?i)(cemal|turkcan|türkcan)`)
	home := regexp.MustCompile(`(?i)/(home|Users)/[a-z][a-z0-9_-]+/`)

	var hits []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "personal_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		where := strings.TrimPrefix(path, root+"/")
		if found := named.Find(source); found != nil {
			hits = append(hits, where+" names "+string(found))
		}
		if found := home.Find(source); found != nil {
			hits = append(hits, where+" hardcodes "+string(found))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(hits) > 0 {
		t.Fatalf("this repository is meant to belong to whoever checks it out: %s",
			strings.Join(hits, "; "))
	}
}
