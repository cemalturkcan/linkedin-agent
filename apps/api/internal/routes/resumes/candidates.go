package resumes

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	maxDepth      = 3
	maxVisits     = 400
	maxPDFs       = 500
	maxCandidates = 12
)

type Candidate struct {
	Dir    string `json:"dir"`
	Count  int    `json:"count"`
	Sample string `json:"sample"`
}

func Candidates() []Candidate {
	found := make([]Candidate, 0, maxCandidates)
	seen := make(map[string]struct{}, maxCandidates)
	for _, dir := range candidateRoots() {
		if _, present := seen[dir]; present {
			continue
		}
		seen[dir] = struct{}{}
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		count, sample := countPDFs(dir)
		if count > 0 {
			found = append(found, Candidate{Dir: dir, Count: count, Sample: sample})
		}
	}
	sort.Slice(found, func(first, second int) bool {
		if found[first].Count != found[second].Count {
			return found[first].Count > found[second].Count
		}
		return found[first].Dir < found[second].Dir
	})
	if len(found) > maxCandidates {
		found = found[:maxCandidates]
	}
	return found
}

func candidateRoots() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	roots := []string{
		filepath.Join(home, "cv"),
		filepath.Join(home, "Documents"),
	}
	projects, err := os.ReadDir(filepath.Join(home, "code"))
	if err != nil {
		return roots
	}
	for _, project := range projects {
		if project.IsDir() && !strings.HasPrefix(project.Name(), ".") {
			roots = append(roots, filepath.Join(home, "code", project.Name(), "out"))
		}
	}
	return roots
}

type visit struct {
	dir   string
	depth int
}

func countPDFs(root string) (int, string) {
	queue := []visit{{dir: root}}
	visits := 0
	count := 0
	sample := ""

	for len(queue) > 0 && visits < maxVisits && count < maxPDFs {
		current := queue[0]
		queue = queue[1:]
		visits++

		entries, err := os.ReadDir(current.dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}
			if entry.Type().IsRegular() {
				if !isPDF(entry.Name()) {
					continue
				}
				count++
				if sample == "" {
					sample = relative(root, filepath.Join(current.dir, entry.Name()))
				}
				continue
			}
			if entry.IsDir() && current.depth+1 < maxDepth {
				queue = append(queue, visit{dir: filepath.Join(current.dir, entry.Name()), depth: current.depth + 1})
			}
		}
	}
	return count, sample
}

func relative(root, path string) string {
	shortened, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return shortened
}
