package linkedin

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var ErrJobSourceUnavailable = errors.New("recommendation source was not offered by LinkedIn")

var recommendationOrigins = map[string]string{
	"preferences":   "PREFERENCES_LANDING",
	"profile_match": "QUALIFICATION_LANDING",
}

func jobListScript(limit int, source string) string {
	return "() => (" + jobsExpression + ")((" + jobCardExpression + "), " + strconv.Itoa(limit) + ", " + strconv.Quote(source) + ")"
}

// JobSourceContext holds the native parameters for one observed Show all link.
// It is never sent in cleartext in an API response or accepted from a caller.
type JobSourceContext struct {
	Source, Origin, Keywords, Locations, Seed string
}

func (c Client) resolveJobSource(ctx context.Context, source string) (JobSourceContext, error) {
	reader := c.Reader
	if source == "profile_match" {
		reader = renderReader(reader)
	}
	link, err := read[struct {
		JobSourceContext
		Unavailable bool
	}](ctx, reader, "/jobs/", jobListScript(0, source))
	if err != nil {
		return JobSourceContext{}, err
	}
	if link.Unavailable {
		return JobSourceContext{}, ErrJobSourceUnavailable
	}
	return link.JobSourceContext, nil
}

func (link JobSourceContext) validFor(source string) bool {
	origin, known := recommendationOrigins[source]
	if !known || link.Source != source || strings.TrimPrefix(link.Origin, "JobSearchOrigin_") != origin ||
		!utf8.ValidString(link.Keywords) || strings.TrimSpace(link.Keywords) == "" || len(link.Keywords) > 1000 ||
		strings.ContainsFunc(link.Keywords, unicode.IsControl) ||
		!validIDList(strings.Split(link.Locations, ","), locationIDPattern, 20) ||
		!validIDList(strings.Split(link.Seed, ","), companyIDPattern, 50) {
		return false
	}
	return true
}

func (link JobSourceContext) searchPath(page int) string {
	args := url.Values{"keywords": {link.Keywords}, "origin": {link.Origin}, "geoId": {link.Locations}}
	if page == 1 {
		args.Set("originToLandingJobPostings", link.Seed)
	} else {
		// The native next-page button retains the origin but removes the
		// posting IDs used to seed the first page.
		args.Set("start", strconv.Itoa((page-1)*searchPageSize))
		args.Set("origin", recommendationOrigins[link.Source])
	}
	return "/jobs/search-results/?" + args.Encode()
}
