package runner

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalidRepoURL = errors.New("repo URL must be https://github.com/<owner>/<repo>")
	ownerRepo         = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
)

func ValidateRepoURL(raw string) (string, error) {
	if strings.ContainsAny(raw, "\\\r\n\t") {
		return "", ErrInvalidRepoURL
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "github.com") || u.Port() != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrInvalidRepoURL
	}
	p := strings.Trim(u.EscapedPath(), "/")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || strings.Contains(p, "%") || !validComponent(parts[0], false) || !validComponent(parts[1], true) {
		return "", ErrInvalidRepoURL
	}
	return "https://github.com/" + parts[0] + "/" + strings.TrimSuffix(parts[1], ".git"), nil
}
func validComponent(s string, repo bool) bool {
	if repo {
		s = strings.TrimSuffix(s, ".git")
	}
	return s != "" && !strings.HasSuffix(s, ".") && ownerRepo.MatchString(s)
}
