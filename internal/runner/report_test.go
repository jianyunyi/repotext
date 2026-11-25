package runner

import (
	"strings"
	"testing"

	"repotest/internal/domain"
)

func TestOfflineReportDistinguishesPackageResults(t *testing.T) {
	output := strings.Join([]string{
		`{"Action":"start","Package":"example.test/pass"}`,
		`{"Action":"output","Package":"example.test/pass","Test":"TestExample","Output":"? [no test files]\n"}`,
		`{"Action":"pass","Package":"example.test/pass","Test":"TestExample"}`,
		`{"Action":"pass","Package":"example.test/pass","Elapsed":0.125}`,
		`{"Action":"start","Package":"example.test/empty"}`,
		`{"Action":"output","Package":"example.test/empty","Output":"?\texample.test/empty\t[no test files]\n"}`,
		`{"Action":"skip","Package":"example.test/empty"}`,
		`{"Action":"build-fail","ImportPath":"example.test/bad"}`,
		`{"Action":"fail","Package":"example.test/bad","FailedBuild":"example.test/bad"}`,
		`{"Action":"start","Package":"example.test/partial"}`,
	}, "\n")
	report := offlineReport(domain.Task{ID: "task"}, "go1.27.1", Result{Output: []byte(output), Truncated: true})
	if report.TaskID != "task" || report.GOOS != "linux" || report.GoVersion != "go1.27.1" || !report.Truncated || report.NoTests {
		t.Fatalf("report metadata: %+v", report)
	}
	if len(report.Packages) != 4 {
		t.Fatalf("packages: %+v", report.Packages)
	}
	for i, want := range []string{"passed", "no_tests", "build_failed", "incomplete"} {
		if got := report.Packages[i].Status; got != want {
			t.Fatalf("package %d status = %q, want %q", i, got, want)
		}
	}
	if report.Packages[0].ElapsedMS != 125 {
		t.Fatalf("elapsed: %+v", report.Packages[0])
	}
}
