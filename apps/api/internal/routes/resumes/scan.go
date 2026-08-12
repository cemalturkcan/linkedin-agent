package resumes

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	maxRoles    = 200
	codeLetters = 2
)

var separators = regexp.MustCompile(`[^a-zA-Z0-9]+`)

type Resume struct {
	Code      string            `json:"code"`
	Label     string            `json:"label"`
	Role      string            `json:"role"`
	Languages []string          `json:"languages"`
	Files     map[string]string `json:"-"`
}

type FolderError struct {
	Path   string
	Reason string
}

func (e *FolderError) Error() string {
	return "the cv folder " + e.Path + " is " + e.Reason
}

func Expand(input string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err == nil {
		if raw == "~" {
			return home
		}
		if strings.HasPrefix(raw, "~"+string(os.PathSeparator)) {
			raw = filepath.Join(home, raw[2:])
		}
	}
	absolute, err := filepath.Abs(raw)
	if err != nil {
		return raw
	}
	return absolute
}

func Scan(dir string) ([]Resume, error) {
	root := Expand(dir)
	if root == "" {
		return nil, &FolderError{Path: dir, Reason: "not a path"}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &FolderError{Path: root, Reason: "not there"}
		}
		return nil, &FolderError{Path: root, Reason: "unreadable"}
	}

	byRole := map[string]map[string]string{}
	order := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.Type().IsRegular() {
			collectFile(root, entry.Name(), byRole, &order)
			continue
		}
		if entry.IsDir() {
			collectFolder(root, entry.Name(), byRole, &order)
		}
	}

	sort.Strings(order)
	taken := make(map[string]struct{}, len(order))
	resumes := make([]Resume, 0, len(order))
	for _, role := range order {
		files := byRole[role]
		languages := make([]string, 0, len(files))
		for language := range files {
			languages = append(languages, language)
		}
		sort.Strings(languages)
		code := assignCode(role, taken)
		taken[code] = struct{}{}
		resumes = append(resumes, Resume{
			Code:      code,
			Label:     prettify(role),
			Role:      role,
			Languages: languages,
			Files:     files,
		})
		if len(resumes) >= maxRoles {
			break
		}
	}
	return resumes, nil
}

func collectFile(root, name string, byRole map[string]map[string]string, order *[]string) {
	if !isPDF(name) {
		return
	}
	label := stem(name)
	role := withoutLanguage(label)
	if role == "" {
		return
	}
	language := languageOfSuffix(label)
	if language == "" {
		language = DefaultLanguage
	}
	record(byRole, order, role, language, filepath.Join(root, name))
}

func collectFolder(root, name string, byRole map[string]map[string]string, order *[]string) {
	role := withoutLanguage(name)
	if role == "" {
		return
	}
	folderLanguage := languageOfSuffix(name)
	entries, err := os.ReadDir(filepath.Join(root, name))
	if err != nil {
		return
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") || !isPDF(entry.Name()) {
			continue
		}
		files = append(files, entry.Name())
	}
	sort.Strings(files)
	for _, file := range files {
		record(
			byRole,
			order,
			role,
			languageOfFile(stem(file), folderLanguage),
			filepath.Join(root, name, file),
		)
	}
}

func record(byRole map[string]map[string]string, order *[]string, role, language, path string) {
	files, present := byRole[role]
	if !present {
		files = map[string]string{}
		byRole[role] = files
		*order = append(*order, role)
	}
	if _, taken := files[language]; !taken {
		files[language] = path
	}
}

func isPDF(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".pdf")
}

func stem(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

func prettify(role string) string {
	words := separators.Split(role, -1)
	titled := make([]string, 0, len(words))
	for _, word := range words {
		if word == "" {
			continue
		}
		letters := []rune(word)
		letters[0] = unicode.ToUpper(letters[0])
		titled = append(titled, string(letters))
	}
	return strings.Join(titled, " ")
}

func assignCode(role string, taken map[string]struct{}) string {
	for _, candidate := range codeCandidates(role) {
		if len(candidate) != codeLetters {
			continue
		}
		if _, used := taken[candidate]; !used {
			return candidate
		}
	}
	for number := 1; ; number++ {
		fallback := "R" + strconv.Itoa(number)
		if _, used := taken[fallback]; !used {
			return fallback
		}
	}
}

func codeCandidates(role string) []string {
	letters := strings.ToUpper(separators.ReplaceAllString(role, ""))
	words := slices.DeleteFunc(separators.Split(role, -1), func(word string) bool {
		return word == ""
	})
	candidates := make([]string, 0, len(letters)+len(words)+8)

	if len(letters) >= codeLetters {
		candidates = append(candidates, letters[:codeLetters])
	}
	for position := 1; position < len(words); position++ {
		candidates = append(candidates, strings.ToUpper(words[0][:1]+words[position][:1]))
	}
	for position := 1; position < len(letters); position++ {
		candidates = append(candidates, letters[:1]+letters[position:position+1])
	}
	head := "R"
	if letters != "" {
		head = letters[:1]
	}
	for digit := 2; digit <= 9; digit++ {
		candidates = append(candidates, head+strconv.Itoa(digit))
	}
	return candidates
}
