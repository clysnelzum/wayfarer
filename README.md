# Wayfarer

Wayfarer is a local, read-only Go proxy for information visible in an authenticated LinkedIn browser session. It exposes a versioned HTTP/JSON API while keeping authentication in Chrome. It does not read, export, store, or relay LinkedIn cookies.

## Run

Requires Go 1.22+ and a Chrome instance with an active LinkedIn session and local remote debugging enabled. Wayfarer connects directly to Chrome over CDP; it does not need the Chrome DevTools MCP server at runtime.

1. In Chrome 144+, enable **Remote debugging** at `chrome://inspect/#remote-debugging`. Use the browser-level WebSocket URL for that instance and approve the connection in Chrome if prompted.
2. Set `WAYFARER_API_TOKEN` to a random secret of at least 32 characters. This is a **Wayfarer** token, not a LinkedIn token.
3. Set `WAYFARER_CHROME_WS` to the browser-level loopback WebSocket URL, then run `go run ./cmd/wayfarer`.

For regular Chrome on Windows, the browser-level URL can be assembled from `DevToolsActivePort` in the Chrome user data directory. If Chrome uses a custom data directory, adjust this example to point at it:

```powershell
$port, $path = Get-Content "$env:LOCALAPPDATA\Google\Chrome\User Data\DevToolsActivePort" -TotalCount 2
$env:WAYFARER_CHROME_WS = "ws://127.0.0.1:${port}${path}"
# Set WAYFARER_API_TOKEN separately, then:
go run ./cmd/wayfarer
```

Chrome may require approval for a new debugger connection to its default profile. Approval granted to another tool, such as Chrome DevTools MCP, does not automatically grant Wayfarer access. Wayfarer keeps up to two debugger connections and reuses healthy connections across requests. It still opens a separate temporary tab for every request. For unattended use, start a separate Chrome instance with a dedicated, non-default `--user-data-dir` and a loopback `--remote-debugging-port`, then sign in to LinkedIn there. Do not copy cookies or expose the debugger outside the local machine.

The API listens on `127.0.0.1:8765` by default. Use `-listen` to choose another loopback address or port. All routes require `Authorization: Bearer <WAYFARER_API_TOKEN>`. For example:

```powershell
Invoke-RestMethod 'http://127.0.0.1:8765/v1/jobs?q=DevOps' `
  -Headers @{ Authorization = "Bearer $env:WAYFARER_API_TOKEN" }
```

## HTTP API

| Route | Response |
| --- | --- |
| `GET /healthz` | `{"status":"ok"}`. Verifies that the API is up, not that Chrome or LinkedIn is reachable. |
| `GET /v1/jobs` | Recommended jobs with `items` and visible recommendation `sections` when their membership is identifiable. Each item has `id`, `title`, `company`, `location`, and `url`. |
| `GET /v1/jobs?q=DevOps&page=2` | Keyword search with typed filters, `items`, `pagination`, and search metadata. `page` defaults to 1. |
| `GET /v1/jobs/locations?q=Buenos+Aires` | LinkedIn location autocomplete. Pass the returned `id` as `geo_id` in a job search. |
| `GET /v1/jobs/companies?q=Microsoft` | LinkedIn job-search company autocomplete. Returns organization IDs, names, and industry labels when present. Use the IDs as `company_id` or `company_ids`. |
| `GET /v1/jobs/{id}` | Posting description, company slug, location, workplace and employment type, application method and state, displayed activity counts, and related jobs where available. |
| `GET /v1/feed` | Personalized feed in the sort order active in Chrome. Omit `limit` for the initial batch, or pass `?limit=1` through `?limit=30` to scan further. |
| `GET /v1/profiles/{slug}` | Personal profile: `slug`, `url`, and `name`, plus top-card details (badges, counts, `open_to_work`, `hiring`), `about`, `top_skills`, `featured`, `activity`, `experience`, `education`, `certifications`, `skills`, `languages`, `projects`, `volunteering`, `publications`, `honors`, `recommendations`, and `interests` when visible. |
| `GET /v1/companies/{slug}` | Company profile: `slug`, `url`, and `name`, plus `id`, top-card badges/counts, overview details, `highlights`, `workplace_policy`, `commitments`, `locations`, `affiliated_pages`, and `similar_pages` when visible. |

For service integrations, use the [OpenAPI contract](docs/openapi.json). A `slug` is the segment after `/in/` or `/company/` in a LinkedIn URL, not a display name or a full URL. Optional profile fields are omitted when unavailable. The service is loopback-only; remote consumers should use an authenticated HTTPS gateway rather than exposing Chrome's debugger or sharing its endpoint.

Feed cards include an opaque `id`, rendered `text`, and a separate `content` body. Author, permalink, relative publication label, hashtags, media, exact engagement counts, and a sponsored marker are returned when detected. `has_more` is only present when there is evidence of more cards; it is not a continuation token. `load_status` distinguishes the initial batch, reaching the requested limit, hitting the time/load budget, seeing no more controls, and a load that did not advance. Requests start fresh and may return fewer items than requested. Media URLs can expire.

### Job search and detail

Example: `GET /v1/jobs?q=DevOps&geo_id=90009870&workplace=remote,hybrid&posted_within=week`.

| Filter | Accepted values |
| --- | --- |
| `geo_id` | Numeric LinkedIn geography ID from `/v1/jobs/locations`. |
| `company_id` or `company_ids` | Numeric LinkedIn organization ID or up to 20 comma-separated IDs from `/v1/jobs/companies`, not company slugs. Use only one of these parameters. |
| `posted_within` | `day`, `week`, `month` |
| `workplace` | `remote`, `hybrid`, `onsite` |
| `experience` | `entry`, `senior`, `manager`, `director`, `executive` |
| `employment_type` | `full_time`, `part_time`, `contract`, `internship`, `volunteer` |
| `suggested_facet_ids` | Up to 12 comma-separated IDs from `search.suggested_facets`. Reuse them with the search that returned them. |
| `easy_apply`, `in_network`, `verified`, `under_10_applicants` | `true` to enable the corresponding LinkedIn filter; `false` leaves it off. |

Company autocomplete preserves LinkedIn's suggestion order. An explicit empty picker response returns `{"items":[],"status":"no_suggestions_observed"}`; an unrecognized response is an upstream error, not an empty result.

All filters require `q`. Workplace, experience, and employment type accept comma-separated values. Pass `pagination.next_page` back with the same `q` and filters. The API accepts pages 1–100 and returns up to 25 cards per page; the order may change between calls. An explicitly empty page whose offset meets or exceeds LinkedIn's positive, unqualified result total returns `200` with `pagination.out_of_range: true`. An empty page with no count, a zero count, or an approximate count does not establish this condition.

When available, `search.suggested_facets` lists LinkedIn's query-specific filters in toolbar order. Each entry includes `id`, a localized `label`, and `selected` as shown by LinkedIn. For example, if a DevOps search suggests AWS with ID `230001:277008`, use `GET /v1/jobs?q=DevOps&suggested_facet_ids=230001%3A277008`. Suggestions can change with the query, account, or selected filters; the field is omitted when none can be identified.

LinkedIn may show similar jobs when no exact matches are available. Wayfarer preserves those results and marks the response `search.match_mode: "broadened"` when LinkedIn explicitly says so. A response marked `results` does not certify that every card matches every filter. When shown, `search.result_count` is accompanied by a qualifier such as `exact` or `more_than`, and `search.result_count_label` preserves the displayed label. `search.notice` carries LinkedIn's explanation when displayed. If LinkedIn changes the requested keyword, page, or supported filters in the final URL, the API returns `502` with `upstream_search_changed` rather than silently serving a different search. See the [jobs parity checks](docs/jobs-parity.md).

In detail responses, `application_activity.kind` distinguishes clicks on an external Apply button from applications. `application_activity.qualifier` preserves whether the number is exact or a lower bound. `viewer_application` reports only the signed-in account's explicitly shown status. Publication and submission labels are relative text, not fabricated timestamps. A listing that shows its description but no longer accepts applications can still return `200` with `application_status: "closed"`.

Responses are JSON and are not cached. Errors use `{"error":{"code":"…","message":"…"}}`. An unreachable or unapproved Chrome debugger returns `503 upstream_debugger_unavailable`; an expired or challenged LinkedIn session returns `503 upstream_auth_required` when detected. An expected page that does not render in time returns `504`. The proxy exposes no write endpoints and does not click Apply, Save, Like, or Post. Normal page views may still be recorded by LinkedIn.

## Scope

This is not a complete LinkedIn API. Personal profiles map the rendered profile page, including scroll-triggered lazy sections (`experience`, `education`, `certifications`, `skills`, `languages`, `projects`, `volunteering`, `publications`, `honors`, `recommendations`, `activity`, `featured`, and `interests`). Company profiles use the About page and sidebar cards (`top card`, `overview` details, associated members, `workplace_policy`, `commitments`, `locations`, `affiliated_pages`/showcase pages, and `similar_pages`). There are no messaging or contact-info endpoints. Job search supports pagination. The feed supports bounded in-page loading but not continuation across HTTP requests, fully expanded comment threads, or write actions. Job recommendations are still limited to initially rendered items. Extraction currently targets the observed Spanish and English interface. The `/v1` contract is independent of LinkedIn's markup, but upstream layout changes may require adapter updates. The API uses a temporary background tab per request and closes it before reusing its debugger connection; it does not reuse or navigate the user's open tabs. An interrupted process may leave a temporary tab behind.

- `cmd/wayfarer/`: server setup
- `internal/api/`: versioned routes and response handling
- `internal/chrome/`: minimal CDP transport and temporary-tab lifecycle
- `internal/linkedin/`: LinkedIn page adapters and typed response models
- [`docs/upstream.md`](docs/upstream.md): observed upstream data sources and remaining research

Run unit tests with `go test ./...` when Go is available. Run the dependency-free extractor fixtures with `node --test tests/jobs-extractors.test.cjs tests/job-company-records.test.cjs tests/company-extractor.test.cjs tests/profile-extractor.test.cjs` (or `node --test tests/*.test.cjs`).
