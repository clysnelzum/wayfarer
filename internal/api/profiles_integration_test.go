package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
	"github.com/gorilla/websocket"
)

func TestProfilesAndCompaniesHTTPThroughCDP(t *testing.T) {
	richProfileValue := `{"state":"ok","value":{"profile":{
		"slug":"example-person",
		"url":"ignored",
		"name":"Example Person",
		"pronouns":"they/them",
		"verified":true,
		"premium":true,
		"top_voice":true,
		"connection_degree":"2nd",
		"headline":"Staff Engineer",
		"location":"Buenos Aires, Argentina",
		"followers_label":"1234 seguidores",
		"followers_count":1234,
		"connections_label":"500+ contactos",
		"current_company_label":"Example Co",
		"education_label":"UBA",
		"website":"https://example.dev/",
		"open_to_work":true,
		"about":"Building reliable systems.",
		"top_skills":["Go","Distributed Systems"],
		"featured":[{"title":"Architecture Notes","type":"Article","url":"https://example.dev/arch"}],
		"activity":[{"url":"https://www.linkedin.com/feed/update/urn:li:activity:1/","published_label":"1d","text":"New post","author_name":"Example Person","author_slug":"example-person"}],
		"experience":[{"title":"Staff Engineer","company":"Example Co","company_slug":"example-co","company_url":"https://www.linkedin.com/company/example-co/","employment_type":"Full-time","date_range":"2022 - Present","current":true}],
		"education":[{"school":"UBA","school_slug":"uba","school_url":"https://www.linkedin.com/school/uba/","degree":"CS","date_range":"2015 - 2020"}],
		"certifications":[{"name":"CKA","issuer":"CNCF","issue_date":"2023"}],
		"skills":[{"name":"Go","endorsements_label":"15 endorsements"}],
		"languages":[{"name":"Spanish","proficiency":"Native"}],
		"projects":[{"title":"Wayfarer","date_range":"2026","url":"https://example.dev/wayfarer"}],
		"volunteering":[{"role":"Mentor","organization":"Code Club","cause":"Education"}],
		"publications":[{"title":"Systems Design","publisher":"Example Press","date":"2025"}],
		"honors":[{"title":"Best Paper","issuer":"Conf","date":"2024"}],
		"recommendations":[{"author_name":"Ada Lovelace","author_slug":"ada-lovelace","author_url":"https://www.linkedin.com/in/ada-lovelace/","relationship":"Worked together","text":"Strong engineer."}],
		"interests":[{"name":"Microsoft","kind":"company","slug":"microsoft","url":"https://www.linkedin.com/company/microsoft/","followers_label":"23 M followers"}]
	}}}`

	richCompanyValue := `{"state":"ok","value":{"company":{
		"slug":"microsoft",
		"url":"ignored",
		"name":"Microsoft",
		"id":"1035",
		"tagline":"Empower every person.",
		"description":"Microsoft enables digital transformation.",
		"website":"https://news.microsoft.com/",
		"industry":"Software Development",
		"company_size":"+10.001 empleados",
		"associated_members_count":231102,
		"associated_members_label":"231.102 miembros asociados",
		"followers_count":23000000,
		"followers_label":"23 M seguidores",
		"employees_label":"231 mil empleados",
		"headquarters":"Redmond, Washington",
		"founded":"1975",
		"company_type":"Public Company",
		"specialties":"Business Software, Developer Tools",
		"verified":true,
		"verified_page":"10 de octubre de 2023",
		"premium":true,
		"highlights":["Great Place to Work"],
		"workplace_policy":{"title":"Trabajo híbrido","description":"Flexible hybrid work","default_presence":"Hybrid","benefits":["Health insurance"]},
		"commitments":[{"title":"Sustainability","description":"Carbon negative by 2030"}],
		"locations_count":45,
		"locations":[{"label":"Redmond","address":"One Microsoft Way, Redmond, WA","is_headquarters":true}],
		"affiliated_pages":[{"slug":"github","url":"https://www.linkedin.com/company/github/","name":"GitHub","industry":"Software Development"}],
		"similar_pages":[{"slug":"google","url":"https://www.linkedin.com/company/google/","name":"Google","industry":"Software Development"}]
	}}}`

	for _, tc := range []struct {
		name           string
		apiPath        string
		wantNavPath    string
		wantExprMarker string
		value          string
		status         int
		wantSubstrings []string
	}{
		{
			name:           "profile 200 rich",
			apiPath:        "/v1/profiles/example-person",
			wantNavPath:    "/in/example-person/",
			wantExprMarker: "com.linkedin.sdui.profile.card.ref",
			value:          richProfileValue,
			status:         http.StatusOK,
			wantSubstrings: []string{
				`"url":"https://www.linkedin.com/in/example-person/"`,
				`"pronouns":"they/them"`,
				`"followers_count":1234`,
				`"experience":[`,
				`"recommendations":[`,
				`"interests":[`,
			},
		},
		{
			name:           "company 200 rich",
			apiPath:        "/v1/companies/microsoft",
			wantNavPath:    "/company/microsoft/about/",
			wantExprMarker: "org-top-card",
			value:          richCompanyValue,
			status:         http.StatusOK,
			wantSubstrings: []string{
				`"url":"https://www.linkedin.com/company/microsoft/"`,
				`"id":"1035"`,
				`"associated_members_count":231102`,
				`"workplace_policy":{`,
				`"commitments":[`,
				`"affiliated_pages":[`,
			},
		},
		{
			name:           "profile 404 not found",
			apiPath:        "/v1/profiles/missing",
			wantNavPath:    "/in/missing/",
			wantExprMarker: "com.linkedin.sdui.profile.card.ref",
			value:          `{"state":"ok","value":{"not_found":true}}`,
			status:         http.StatusNotFound,
			wantSubstrings: []string{`"code":"profile_not_found"`},
		},
		{
			name:           "company 404 not found",
			apiPath:        "/v1/companies/missing",
			wantNavPath:    "/company/missing/about/",
			wantExprMarker: "org-top-card",
			value:          `{"state":"ok","value":{"not_found":true}}`,
			status:         http.StatusNotFound,
			wantSubstrings: []string{`"code":"company_not_found"`},
		},
		{
			name:           "profile 503 auth required",
			apiPath:        "/v1/profiles/example-person",
			wantNavPath:    "/in/example-person/",
			wantExprMarker: "com.linkedin.sdui.profile.card.ref",
			value:          `{"state":"auth"}`,
			status:         http.StatusServiceUnavailable,
			wantSubstrings: []string{`"code":"upstream_auth_required"`},
		},
		{
			name:           "company 503 auth required",
			apiPath:        "/v1/companies/microsoft",
			wantNavPath:    "/company/microsoft/about/",
			wantExprMarker: "org-top-card",
			value:          `{"state":"auth"}`,
			status:         http.StatusServiceUnavailable,
			wantSubstrings: []string{`"code":"upstream_auth_required"`},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			closed := make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				upgrader := websocket.Upgrader{}
				ws, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Errorf("CDP handshake: %v", err)
					return
				}
				defer ws.Close()
				_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
				for _, method := range []string{"Target.createTarget", "Target.attachToTarget", "Emulation.setFocusEmulationEnabled", "Emulation.setDeviceMetricsOverride", "Page.navigate", "Runtime.evaluate", "Target.closeTarget"} {
					var request struct {
						ID     int
						Method string
						Params struct {
							URL, TargetID, Expression string
							AwaitPromise              bool
						}
					}
					if err := ws.ReadJSON(&request); err != nil || request.Method != method {
						t.Errorf("expected %s, got %s (%v)", method, request.Method, err)
						return
					}
					var result any
					switch method {
					case "Target.createTarget":
						result = map[string]string{"targetId": "target"}
					case "Target.attachToTarget":
						result = map[string]string{"sessionId": "session"}
					case "Emulation.setFocusEmulationEnabled", "Emulation.setDeviceMetricsOverride":
						result = struct{}{}
					case "Page.navigate":
						u, err := url.Parse(request.Params.URL)
						if err != nil || u.Host != "www.linkedin.com" || u.Path != tc.wantNavPath {
							t.Errorf("unexpected upstream navigation: %s (want path %s)", request.Params.URL, tc.wantNavPath)
						}
						result = map[string]string{"frameId": "frame"}
					case "Runtime.evaluate":
						if !request.Params.AwaitPromise || !strings.Contains(request.Params.Expression, tc.wantExprMarker) {
							t.Errorf("expected extractor containing %q with AwaitPromise=true", tc.wantExprMarker)
						}
						result = map[string]any{"result": map[string]any{"value": json.RawMessage(tc.value)}}
					case "Target.closeTarget":
						if request.Params.TargetID != "target" {
							t.Error("attempted to close the wrong target")
						}
						closed <- struct{}{}
						return
					}
					if err := ws.WriteJSON(map[string]any{"id": request.ID, "result": result}); err != nil {
						t.Errorf("write CDP reply: %v", err)
						return
					}
				}
			}))
			defer upstream.Close()
			browser, err := chrome.New("ws" + strings.TrimPrefix(upstream.URL, "http") + "/devtools/browser/fixture")
			if err != nil {
				t.Fatal(err)
			}
			defer browser.Close()

			req := httptest.NewRequest(http.MethodGet, tc.apiPath, nil)
			req.Header.Set("Authorization", "Bearer local")
			w := httptest.NewRecorder()
			New("local", linkedin.Client{Reader: browser}).ServeHTTP(w, req)

			if w.Code != tc.status {
				t.Fatalf("unexpected HTTP status %d (want %d): %s", w.Code, tc.status, w.Body)
			}
			body := w.Body.String()
			for _, sub := range tc.wantSubstrings {
				if !strings.Contains(body, sub) {
					t.Fatalf("expected response %s to contain %s", body, sub)
				}
			}
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("temporary browser tab was not closed")
			}
		})
	}
}
