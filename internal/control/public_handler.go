package control

import (
	"errors"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"repotest/internal/domain"
)

var publicReportPage = template.Must(template.ParseFS(pageFS, "web/public.html"))

type publicPageData struct {
	TaskID             string
	Title              string
	Description        string
	RepoName           string
	Expired            bool
	Unavailable        bool
	UnavailableMessage string
}

func (a API) PublicHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /r/{id}", a.publicPage)
	mux.HandleFunc("GET /api/v1/public/reports/{id}", a.publicReport)
	mux.HandleFunc("POST /api/v1/public/previews", a.publicPreview)
	mux.HandleFunc("POST /api/v1/public/tasks", a.publicConfirm)
	mux.HandleFunc("POST /api/v1/public/tasks/{id}/cancel", a.publicCancel)
	mux.HandleFunc("DELETE /api/v1/public/tasks/{id}", a.publicDelete)
	assets := map[string]string{
		"/assets/report.js":  "text/javascript; charset=utf-8",
		"/assets/report.css": "text/css; charset=utf-8",
		"/assets/public.js":  "text/javascript; charset=utf-8",
		"/assets/public.css": "text/css; charset=utf-8",
	}
	for assetPath, contentType := range assets {
		assetPath, contentType := assetPath, contentType
		mux.HandleFunc("GET "+assetPath, func(w http.ResponseWriter, r *http.Request) {
			body, err := pageFS.ReadFile("web/" + strings.TrimPrefix(assetPath, "/assets/"))
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		})
	}
	return publicSecurityHeaders(mux)
}

func publicSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; object-src 'none'; frame-ancestors 'none'; form-action 'none'")
		next.ServeHTTP(w, r)
	})
}

func (a API) publicReport(w http.ResponseWriter, r *http.Request) {
	response, err := a.loadPublicReport(r.Context(), r.PathValue("id"))
	if err != nil {
		writePublicReportError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a API) publicPage(w http.ResponseWriter, r *http.Request) {
	response, err := a.loadPublicReport(r.Context(), r.PathValue("id"))
	data := publicPageData{TaskID: r.PathValue("id"), Title: "RepoTest 报告", Description: "公开仓库测试报告"}
	if errors.Is(err, errPublicReportExpired) {
		data.Expired = true
		data.Title = "报告已过期 · RepoTest"
		data.Description = "此报告已超过 7 天保留期并被清除。"
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusGone)
		_ = publicReportPage.Execute(w, data)
		return
	}
	if err != nil {
		data.Unavailable = true
		data.Title = "报告暂不可用 · RepoTest"
		statusCode := http.StatusInternalServerError
		switch {
		case errors.Is(err, errPublicReportNotFound):
			data.Description = "报告不存在、已过期或已清除。"
			data.UnavailableMessage = data.Description
			statusCode = http.StatusNotFound
		case errors.Is(err, errPublicStorage):
			data.Description = "报告暂时无法读取，请稍后重试。"
			data.UnavailableMessage = data.Description
			statusCode = http.StatusServiceUnavailable
		default:
			data.Description = "报告暂时无法显示，请稍后重试。"
			data.UnavailableMessage = data.Description
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(statusCode)
		_ = publicReportPage.Execute(w, data)
		return
	}
	data.RepoName = strings.TrimPrefix(response.Task.RepoURL, "https://github.com/")
	if data.RepoName == "" {
		data.RepoName = "公开仓库"
	}
	data.Title = data.RepoName + " · RepoTest 报告"
	data.Description = publicTaskStatusLabel(response.Task.Status)
	if response.Report != nil && response.Report.Summary != nil {
		summary := response.Report.Summary
		data.Description = "通过 " + strconv.Itoa(summary.Passed) + " · 失败 " + strconv.Itoa(summary.Failed) + " · 跳过 " + strconv.Itoa(summary.Skipped)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = publicReportPage.Execute(w, data)
}

func publicTaskStatusLabel(status domain.Status) string {
	switch status {
	case domain.StatusQueued:
		return "任务排队中"
	case domain.StatusRunning:
		return "任务运行中"
	case domain.StatusSucceeded:
		return "测试完成"
	case domain.StatusFailed:
		return "测试失败"
	case domain.StatusTimeout:
		return "测试超时"
	case domain.StatusCancelled:
		return "任务已取消"
	case domain.StatusError:
		return "执行错误"
	default:
		return "报告状态未知"
	}
}

func writePublicReportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPublicReportNotFound):
		writeError(w, http.StatusNotFound, "report_not_found")
	case errors.Is(err, errPublicReportExpired):
		writeError(w, http.StatusGone, "report_expired")
	case errors.Is(err, errPublicStorage):
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error")
	}
}
