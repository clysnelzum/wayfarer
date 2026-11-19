package api

import "net/http"

func newRouter(h *handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/v1/feed", h.handleFeed)
	mux.HandleFunc("/v1/jobs", h.handleJobs)
	mux.HandleFunc("/v1/jobs/locations", h.handleJobLocations)
	mux.HandleFunc("/v1/jobs/companies", h.handleJobCompanies)
	mux.HandleFunc("/v1/jobs/{id...}", h.handleJob)
	mux.HandleFunc("/v1/profiles/{slug...}", h.handleProfile)
	mux.HandleFunc("/v1/companies/{slug...}", h.handleCompany)
	mux.HandleFunc("/", h.handleNotFound)
	return mux
}
