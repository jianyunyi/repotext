# RepoTest PUBLIC-2 Read-Only Sharing Design

Mode: Full; execution: continuous. Status: PUBLIC-2 implemented and locally verified; public listeners remain disabled.
Baseline: `feat/public-report-s1`; public listeners remain disabled.
Source: the user-provided RepoTest publicization checklist and the approved PUBLIC-2 design.

## Goal and boundary

Add anonymous read-only report sharing at `/r/<task-id>`. This stage adds an independently mountable public handler only. It does not change the current listener configuration, enable public networking, add public submission, tokens, CAPTCHA, rate limiting, queues, or sandbox changes.

The public handler exposes only the report page, a read-only report API, and public static assets. Preview, confirmation, cancellation, deletion, logs, and device-session routes remain outside this handler. Existing task IDs contain 32 lowercase hexadecimal characters generated from 16 cryptographically random bytes; PUBLIC-2 preserves and validates that format.

## Public data contract

`GET /api/v1/public/reports/{id}` returns a task summary and an optional structured report:

- task ID, repository URL, ref, fixed commit, status, phase, and timestamps;
- Go version, GOOS/GOARCH, package/test states, package coverage, failure locations, and summary;
- diagnostics only for failed, build-failed, or incomplete items, capped at 4 KiB each;
- no internal error code/message, complete raw log, or cancellation/deletion credential.

Completed reports older than seven days return `410 report_expired`. Missing or malformed IDs return `404 report_not_found`. Queued/running tasks may return status without a report body.

The JSON API retains those error envelopes. The HTML share page renders an explanatory `410` page for expired reports, a generic `404` page for missing/malformed reports, and a generic `503` page for storage failures. Error pages do not load the report bootstrap or expose internal storage details.

## Page and security boundary

`GET /r/{id}` uses `html/template` for escaped Open Graph metadata and task data. The page loads the existing safe DOM report renderer through same-origin assets. Repository output remains rendered through DOM text nodes, never HTML string concatenation.

Responses set `Cache-Control: no-store`, `X-Content-Type-Options: nosniff`, `Referrer-Policy: no-referrer`, and a restrictive CSP. The page has no cancellation, deletion, log, or device-session controls.

## Acceptance and rollback

- success, failure, running, and expired reports are covered by handler tests;
- the public handler works without loopback, pairing cookies, or CSRF;
- private task error fields and complete logs are absent from the public projection;
- mutation, log, session, unknown, and malformed routes never return success;
- expired, unavailable, and storage-error pages render safe explanatory HTML without write controls;
- existing local/LAN handlers, runner, storage TTL, and report renderer regressions remain passing;
- no public listener or deployment change is made before PUBLIC-5/6 pass.

Rollback removes only the PUBLIC-2 handler, page, assets, tests, and evidence. It does not roll back PUBLIC-1 parsing or modify running services.

## Decision record

2026-11-25: the user approved the isolated public-handler plus report-projection approach because its route table makes it difficult to accidentally expose local mutation or full-log capabilities.

2026-11-26: verified ISSUE-020 with a failing page test, then rendered generic HTML error states while preserving the JSON API contract. Expired-report resubmission remains deferred to PUBLIC-3 because public submission is outside this stage.
