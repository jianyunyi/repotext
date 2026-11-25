package runner

import "testing"

func TestValidateRepoURLRejectsTrailingDot(t *testing.T) {
	if _, err := ValidateRepoURL("https://github.com/acme/repo."); err == nil {
		t.Fatal("expected trailing dot to be rejected")
	}
}
func TestValidateRepoURLAllowsKnownGitSuffix(t *testing.T) {
	got, err := ValidateRepoURL("https://github.com/acme/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://github.com/acme/repo" {
		t.Fatalf("got %q", got)
	}
}
