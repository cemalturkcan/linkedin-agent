package resumes

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func write(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("%PDF-1.4\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func find(t *testing.T, found []Resume, code string) Resume {
	t.Helper()
	for _, resume := range found {
		if resume.Code == code {
			return resume
		}
	}
	t.Fatalf("no resume with code %s in %v", code, found)
	return Resume{}
}

func TestAFolderPerLanguageYieldsEveryLanguage(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"en.pdf", "de.pdf", "fr.pdf"} {
		write(t, filepath.Join(root, "backend", name))
	}

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("roles = %d, want 1 (%v)", len(found), found)
	}
	want := []string{"de", "en", "fr"}
	if !slices.Equal(found[0].Languages, want) {
		t.Fatalf("languages = %v, want %v", found[0].Languages, want)
	}
	if found[0].Role != "backend" || found[0].Label != "Backend" {
		t.Fatalf("role = %q label = %q", found[0].Role, found[0].Label)
	}
}

func TestAFlatFolderReadsTheLanguageSuffix(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "backend.pdf"))
	write(t, filepath.Join(root, "backend-nl.pdf"))

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("roles = %d, want 1 (%v)", len(found), found)
	}
	want := []string{"en", "nl"}
	if !slices.Equal(found[0].Languages, want) {
		t.Fatalf("languages = %v, want %v", found[0].Languages, want)
	}
}

func TestAFolderSuffixNamesTheLanguageForEveryFileInside(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "backend", "doc.pdf"))
	write(t, filepath.Join(root, "backend-tr", "doc.pdf"))
	write(t, filepath.Join(root, "backend-ja", "doc.pdf"))

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("roles = %d, want 1 (%v)", len(found), found)
	}
	want := []string{"en", "ja", "tr"}
	if !slices.Equal(found[0].Languages, want) {
		t.Fatalf("languages = %v, want %v", found[0].Languages, want)
	}
}

func TestCodesAreTwoLettersAndNeverCollide(t *testing.T) {
	root := t.TempDir()
	for _, role := range []string{"java", "java-dotnet", "javascript", "go", "genel"} {
		write(t, filepath.Join(root, role, "cv.pdf"))
	}

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	seen := map[string]string{}
	for _, resume := range found {
		if len(resume.Code) != 2 {
			t.Fatalf("code %q for role %q is not two letters", resume.Code, resume.Role)
		}
		if other, taken := seen[resume.Code]; taken {
			t.Fatalf("code %q is shared by %q and %q", resume.Code, other, resume.Role)
		}
		seen[resume.Code] = resume.Role
	}
	if got := find(t, found, "JD").Role; got != "java-dotnet" {
		t.Fatalf("JD is %q", got)
	}
}

func TestAFolderThatIsNotThereIsNamed(t *testing.T) {
	_, err := Scan(filepath.Join(t.TempDir(), "absent"))
	var folder *FolderError
	if !errors.As(err, &folder) {
		t.Fatalf("err = %v", err)
	}
	if folder.Reason != "not there" {
		t.Fatalf("reason = %q", folder.Reason)
	}
}

func TestAFileNamedCvIsTheDocumentAndNotChuvash(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "backend", "cv.pdf"))

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !slices.Equal(found[0].Languages, []string{"en"}) {
		t.Fatalf("languages = %v", found[0].Languages)
	}
}

func TestASuffixThatIsNotALanguageStaysPartOfTheRole(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "backend-xx", "doc.pdf"))

	found, err := Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if found[0].Role != "backend-xx" {
		t.Fatalf("role = %q", found[0].Role)
	}
	if !slices.Equal(found[0].Languages, []string{"en"}) {
		t.Fatalf("languages = %v", found[0].Languages)
	}
}

func TestUploadNameIsTheNameOnTheFilesWhenTheyAgree(t *testing.T) {
	root := t.TempDir()
	for _, role := range []string{"java", "java-tr", "backend"} {
		if err := os.MkdirAll(filepath.Join(root, role), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, role, "my-cv.pdf"), []byte("%PDF"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	found, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := sharedName(found); got != "my-cv.pdf" {
		t.Fatalf("shared name = %q, want my-cv.pdf", got)
	}

	if err := os.MkdirAll(filepath.Join(root, "mobile"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "mobile", "resume.pdf"), []byte("%PDF"), 0o644); err != nil {
		t.Fatal(err)
	}
	mixed, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := sharedName(mixed); got != "" {
		t.Fatalf("files that disagree named %q, want no name at all", got)
	}
}
