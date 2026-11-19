# Jobs endpoint parity checks

Scope: `GET /v1/jobs` (recommendations and paginated search), job detail, and filter lookup routes. The job tracker and write actions are not part of this work. The current upstream is LinkedIn's authenticated web experience through Chrome, not the official LinkedIn integration API or a direct implementation of the Flagship/RSC wire protocol.

## What the endpoint preserves

- The initial ordering and per-page membership of extractable cards. Recommendation requests with `limit` also collect lazily loaded cards and retain their identifiable section memberships. Responsive DOM copies are deduplicated by job ID. Sponsored or broadened alternatives are not filtered out by Wayfarer.
- The currently selected job ID, pagination controls, approximate counts and their qualifiers, the displayed geography, and notices about broadening or empty results, when present. The search alert's state is reported only when it can be read from the visible switch.
- Recommendation panel titles and references into `items`, when a panel can be identified from its heading and container. A job can appear in more than one panel without duplicating its card in `items`.
- Recognized labels such as verified, viewed, Easy Apply, early applicant, and actively reviewing, plus recognizable compensation, benefits, and network text shown on the card. The absence of an optional label does not prove the opposite.
- The native **Show all** search for exposed recommendation sections, preserving its origin, locations, and first-page seed context without exposing those seed parameters in cleartext. `source_ref` lets a client pin the original section's context instead of resolving the home page again.
- Identifiable query-specific topic and industry filters in toolbar order, with their current checked state. Feed their IDs back through `suggested_facet_ids` to refine the same search.
- The job search company picker's suggestion order and numeric IDs, including different IDs for identical display names. Use the resulting IDs in `company_id` or `company_ids`.
- Job detail includes optional plain-text description blocks for paragraphs, headings, and flat lists. On one authenticated listing, all 31 items across four lists matched the rendered description DOM in order, without expanding the section. Other markup, including nested lists, may still require the plain-text fallback. A US listing exposed `180K USD/yr - 225K USD/yr` in its header and an unchecked similar-jobs alert switch; the extractor preserved both without clicking. A synthetic invalid job ID rendered the Spanish invalid-or-deleted notice without changing the URL. That exact layout is recognized as not found, unlike a generic load error.

## Search identity and failure modes

Before sending a successful search response, Wayfarer compares the final browser URL with the requested keyword, offset, and supported filters. It accepts harmless ordering differences in semantic facet groups/alternatives, and an upstream default location if none was requested. It rejects dropped filters, altered keywords, unexpected supported filters, duplicate values for a URL key, or a different page with `502` / `upstream_search_changed`. The upstream URL and tracking fields are not returned by this check.

Matching URL parameters do **not** prove that all results match those filters. LinkedIn can relax a query while keeping its parameters in the URL. The endpoint distinguishes this only when LinkedIn explicitly indicates it. A loading skeleton is not treated as an empty result.

## Regressions

Run the dependency-free extractor fixtures and Go contract tests:

```text
node --test tests/*.test.cjs
go test ./...
```

The JavaScript fixtures cover observable page states: delayed notices, approximate totals, page mismatch, explicit empty results beyond the last page, overlapping recommendation panels, localized card labels, and location autocomplete (`tests/job-locations.test.cjs`, alongside `tests/feed-extractor.test.cjs` for feed extraction). Extractor scripts live under `internal/linkedin/scripts/*.js` and can be overridden on disk without recompiling Go via `-scripts-dir` or `WAYFARER_SCRIPTS_DIR`. Go tests cover normalization, search identity, and conservatively identifying out-of-range pages. `TestJobsHTTPThroughCDP` also exercises the production handler and Chrome reader against a simulated CDP WebSocket. It checks the upstream URL and tab cleanup, plus the final-page, out-of-range, dropped-filter, authentication, and debugger-unavailable HTTP responses. These tests check the contract, not live LinkedIn compatibility.

With a signed-in, explicitly debuggable local Chrome instance, run the opt-in HTTP smoke test and browser checks. In PowerShell, with `WAYFARER_CHROME_WS` already set:

```powershell
$env:WAYFARER_LIVE_JOBS = '1'
go test ./internal/api -run '^TestLiveJobsHTTP$' -count=1
go test ./internal/linkedin -run '^TestLiveJob(Search|CompanyLookup|SuggestedFacet)$' -count=1
```

A smoke run of the compiled binary against a clean, signed-out Chrome profile verified `200` for health, `401` without a Wayfarer token, and `503 upstream_auth_required` when LinkedIn required login. The live HTTP test against an authenticated Chrome profile subsequently passed. A separate run of the compiled executable returned `200` and 25 cards for each of two sequential requests (DevOps with Under 10 Applicants, and page two of an unfiltered DevOps search). Both reads used one debugger connection. The live Go search audit passed for standard search, page two, combined filters, and Under 10 Applicants; the company-ID and suggested-facet round trips also passed. If `upstream_debugger_unavailable` is returned, confirm the debugger URL and approve Wayfarer's connection rather than relying on another client's approval.

The HTTP smoke test starts the production handler on a loopback socket, checks that an unauthenticated request is rejected, and sends authenticated recommendation and search requests through Chrome to LinkedIn. It verifies HTTP status, response headers, and JSON without relying on a fixed personalized count. The browser checks cover a standard search, page two, a combined filter search, and Under 10 Applicants. They verify the request controls and compares card membership and order against an independent DOM read on the same page, without expecting a fixed result count or order across sessions. It never clicks a listing action. The company lookup test checks the numeric ID returned for Microsoft and passes it back into a new search. The suggested-facet test reapplies the first facet offered for Product Designer and checks that it becomes selected; it skips if the layout/account offers none.

A manual Chrome check observed 25 matching card IDs in the same order for page two and for Under 10 Applicants, and two identifiable recommendation panels with seven cards total. A two-company/remote/week search retained both company IDs and surfaced LinkedIn's own broadened-search notice. The picker returned Microsoft (`1035`) and Mercado Libre (`14226`) with the same suggestion order as the displayed rows after whitespace normalization. Using `1035` in a DevOps search preserved the company filter and reported an explicit empty result in that session. A Product Designer search exposed five suggestions; reusing three IDs retained both facet groups, checked the requested suggestions, and preserved LinkedIn's broadened-result notice. For a DevOps search with Under 10 Applicants, page two contained 25 cards and no next control; pages three and four were explicitly empty while the header still displayed 50 results. An empty page at a large offset in a separate search showed no count, so an out-of-range flag could not be established. Two further authenticated HTTP requests returned 25 cards for `source=preferences&page=2` and for a keyword search with two `geo_ids`. The profile-match source was available during the native click test but absent during the later HTTP run, which returned `404 recommendation_source_unavailable`. A user's preferences tab had the same keywords and locations as the newly inspected home link but different first-page seed jobs. The initial API comparison matched 19 of the user's 25 cards. A fresh native click and two exact URL navigations agreed on all 25 IDs and their order. The HTTP flow for an issued reference returned `200` for recommendations and for pages one and two with the same reference; tests separately check expiry, tampering, and wrong-source rejection. In a US search, the extractor returned 25 cards, including recognized compensation, benefit, and network labels. In a 390-pixel viewport it returned 25 DevOps cards, pagination, and five hydrated facet suggestions. These are session-specific checks, not a general parity claim.

## Remaining differences

- Suggested facets cover the observed topic and industry toolbar controls. Other suggestion groups and layouts are not mapped. Company autocomplete reflects the native picker and does not scan all organizations.
- Native ranking, ads, and personalized experiments can differ by account, layout, locale, or request time. This is not a stable snapshot or a guarantee of all results past the proxy's documented limits.
- The UI adapter does not expose every RSC entity, ranking signal, or internal search response field. See [upstream observations](upstream.md) for what was traced rather than inferring undocumented behavior.
