package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/clysnelzum/wayfarer/internal/chrome"
	"github.com/clysnelzum/wayfarer/internal/linkedin"
	"github.com/gorilla/websocket"
)

func TestJobsHTTPThroughCDP(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   string
		status  int
		payload string
		mode    string
	}{
		{"last page", `{"state":"ok","value":{"items":[{"id":"101","title":"Developer","company":"Example","location":"Remote","url":"https://www.linkedin.com/jobs/view/101/"}],"page":2,"match_mode":"results","query_evidence":{"keywords":["DevOps"],"start":["25"],"f_EA":["true"]}}}`, 200, `"has_more":false`, ""},
		{"search alert off", `{"state":"ok","value":{"items":[{"id":"101","title":"Developer","company":"Example","location":"Remote","url":"https://www.linkedin.com/jobs/view/101/"}],"page":2,"match_mode":"results","alert_enabled":false,"query_evidence":{"keywords":["DevOps"],"start":["25"],"f_EA":["true"]}}}`, 200, `"alert_enabled":false`, ""},
		{"past last page", `{"state":"ok","value":{"items":[],"page":2,"match_mode":"no_results","result_count":25,"result_count_qualifier":"exact","query_evidence":{"keywords":["DevOps"],"start":["25"],"f_EA":["true"]}}}`, 200, `"out_of_range":true`, ""},
		{"dropped filter", `{"state":"ok","value":{"items":[],"page":2,"match_mode":"no_results","query_evidence":{"keywords":["DevOps"],"start":["25"]}}}`, 502, `"code":"upstream_search_changed"`, ""},
		{"auth required", `{"state":"auth"}`, 503, `"code":"upstream_auth_required"`, ""},
		{"no debugger", ``, 503, `"code":"upstream_debugger_unavailable"`, ""},
		{"expanded recommendations", `{"state":"ok","value":{"items":[{"id":"133","title":"Designer","company":"Example","location":"Remote","url":"https://www.linkedin.com/jobs/view/133/"}],"page":0,"recommendation_status":"budget_reached","sections":[{"title":"More jobs","job_ids":["133"]}]}}`, 200, `"recommendation_status":"budget_reached"`, "expanded"},
		{"unavailable job detail", `{"state":"ok","value":{"not_found":true}}`, 404, `"code":"job_not_found"`, "detail"},
		{"job detail alert disabled", `{"state":"ok","value":{"job":{"id":"42","title":"Developer","company":"Example","url":"https://www.linkedin.com/jobs/view/42/","description":"Build software.","similar_job_alert_enabled":false,"compensation_label":"180K USD/yr - 225K USD/yr"}}}`, 200, `"similar_job_alert_enabled":false`, "detail"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/v1/jobs?q=DevOps&page=2&under_10_applicants=true"
			wantURL := "https://www.linkedin.com/jobs/search-results/?f_EA=true&keywords=DevOps&start=25"
			methods := []string{"Target.createTarget", "Target.attachToTarget"}
			switch tc.mode {
			case "expanded":
				path = "/v1/jobs?limit=12"
				wantURL = "https://www.linkedin.com/jobs/"
				methods = append(methods, "Emulation.setFocusEmulationEnabled", "Emulation.setDeviceMetricsOverride")
			case "detail":
				path = "/v1/jobs/42"
				wantURL = "https://www.linkedin.com/jobs/view/42/"
			}
			methods = append(methods, "Page.navigate", "Runtime.evaluate", "Target.closeTarget")
			closed := make(chan struct{}, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.value == "" {
					http.NotFound(w, r)
					return
				}
				upgrader := websocket.Upgrader{}
				ws, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					t.Errorf("CDP handshake: %v", err)
					return
				}
				defer ws.Close()
				_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
				for _, method := range methods {
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
						if request.Params.URL != wantURL {
							t.Errorf("unexpected upstream navigation: %s", request.Params.URL)
						}
						result = map[string]string{"frameId": "frame"}
					case "Runtime.evaluate":
						if !request.Params.AwaitPromise || strings.TrimSpace(request.Params.Expression) == "" {
							t.Error("expected a non-empty asynchronous extractor expression")
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
			client, err := linkedin.NewClient(browser)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer local")
			w := httptest.NewRecorder()
			New("local", client).ServeHTTP(w, req)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.payload) || strings.Contains(w.Body.String(), "query_evidence") {
				t.Fatalf("unexpected HTTP response %d: %s", w.Code, w.Body)
			}
			if tc.value != "" {
				select {
				case <-closed:
				case <-time.After(5 * time.Second):
					t.Fatal("temporary browser tab was not closed")
				}
			}
		})
	}
}
