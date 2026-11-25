package control

import "testing"

func TestWorkKeyIncludesResolvedCommitAndEveryExecutorVersion(t *testing.T) {
	baseIdentity := testExecutionIdentity()
	base, err := baseIdentity.WorkKey()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		mutate func(*ExecutionIdentity)
	}{
		{"repo", func(i *ExecutionIdentity) { i.repoURL = "https://github.com/acme/other" }},
		{"commit", func(i *ExecutionIdentity) { i.commitSHA = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }},
		{"go", func(i *ExecutionIdentity) { i.goVersion = "go1.24.0" }},
		{"runner", func(i *ExecutionIdentity) { i.runnerVersion = "runner-v2" }},
		{"config", func(i *ExecutionIdentity) { i.config = []byte("other-config") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			identity := baseIdentity
			tc.mutate(&identity)
			got, e := identity.WorkKey()
			if e != nil {
				t.Fatal(e)
			}
			if got == base {
				t.Fatal("identity change reused prior work key")
			}
		})
	}
}

func TestWorkKeyRejectsShortCommit(t *testing.T) {
	identity := testExecutionIdentity()
	identity.commitSHA = "abc"
	if _, err := identity.WorkKey(); err == nil {
		t.Fatal("short commit accepted")
	}
}
