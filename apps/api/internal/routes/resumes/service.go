package resumes

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"api/internal/app/clock"
	"api/internal/app/store"
	resumesdb "api/internal/routes/resumes/gen"
)

var (
	ErrNoFolder = errors.New("no cv folder chosen yet, pick one in setup")
	ErrNoPDFs   = errors.New("that folder holds no pdfs, pick the folder that holds your cvs")
	ErrNoResume = errors.New("no resume with that code")
)

type LanguageError struct {
	Code      string
	Language  string
	Available []string
}

func (e *LanguageError) Error() string {
	return "resume " + e.Code + " has no " + e.Language +
		" file, it has " + strings.Join(e.Available, ", ")
}

type Dependencies struct {
	Reads store.DBTX
	Clock clock.Clock
}

type Service struct {
	queries *resumesdb.Queries
	clock   clock.Clock
}

func NewService(dependencies Dependencies) *Service {
	return &Service{
		queries: resumesdb.New(dependencies.Reads),
		clock:   dependencies.Clock,
	}
}

func (s *Service) Folder(ctx context.Context) (string, error) {
	folder, err := s.queries.ResumeFolder(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return folder, nil
}

func (s *Service) SetFolder(ctx context.Context, dir string) (string, []Resume, error) {
	root := Expand(dir)
	info, err := os.Stat(root)
	if err != nil {
		return "", nil, &FolderError{Path: root, Reason: "not there"}
	}
	if !info.IsDir() {
		return "", nil, &FolderError{Path: root, Reason: "not a folder"}
	}
	found, err := Scan(root)
	if err != nil {
		return "", nil, err
	}
	if len(found) == 0 {
		return "", nil, ErrNoPDFs
	}
	if err := s.queries.SaveResumeFolder(ctx, resumesdb.SaveResumeFolderParams{
		Path:      root,
		UpdatedAt: clock.Stamp(s.clock),
	}); err != nil {
		return "", nil, err
	}
	return root, found, nil
}

func (s *Service) List(ctx context.Context) (string, []Resume, error) {
	root, err := s.Folder(ctx)
	if err != nil {
		return "", nil, err
	}
	if root == "" {
		return "", nil, ErrNoFolder
	}
	found, err := Scan(root)
	if err != nil {
		return root, nil, err
	}
	if len(found) == 0 {
		return root, nil, ErrNoPDFs
	}
	return root, found, nil
}

func (s *Service) Languages(ctx context.Context) []string {
	_, found, err := s.List(ctx)
	if err != nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	for _, resume := range found {
		for _, language := range resume.Languages {
			seen[language] = struct{}{}
		}
	}
	languages := make([]string, 0, len(seen))
	for language := range seen {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}

func (s *Service) UploadName(ctx context.Context) string {
	_, found, err := s.List(ctx)
	if err != nil {
		return ""
	}
	return sharedName(found)
}

func sharedName(found []Resume) string {
	shared := ""
	for _, resume := range found {
		for _, path := range resume.Files {
			name := stem(filepath.Base(path))
			if name == "" {
				return ""
			}
			if shared == "" {
				shared = name
				continue
			}
			if shared != name {
				return ""
			}
		}
	}
	if shared == "" {
		return ""
	}
	return shared + ".pdf"
}

type File struct {
	Name  string
	Bytes []byte
}

func (s *Service) File(ctx context.Context, code, language string) (File, error) {
	_, found, err := s.List(ctx)
	if err != nil {
		return File{}, err
	}
	wanted := strings.ToUpper(strings.TrimSpace(code))
	for _, resume := range found {
		if resume.Code != wanted {
			continue
		}
		path, present := resume.Files[strings.ToLower(strings.TrimSpace(language))]
		if !present {
			return File{}, &LanguageError{
				Code:      resume.Code,
				Language:  strings.ToLower(strings.TrimSpace(language)),
				Available: resume.Languages,
			}
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return File{}, &FolderError{Path: path, Reason: "unreadable"}
		}
		return File{Name: filepath.Base(path), Bytes: bytes}, nil
	}
	return File{}, ErrNoResume
}
