package linkedin

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var recommendationOrigins = map[string]string{
	"preferences":   "PREFERENCES_LANDING",
	"profile_match": "QUALIFICATION_LANDING",
}

// jobSourceContext holds the native parameters for one observed Show all link.
// It is encoded as an opaque string in JobSection/JobSearchInfo/JobQuery with json:"-".
type jobSourceContext struct {
	Source    string `json:"source"`
	Origin    string `json:"origin"`
	Keywords  string `json:"keywords"`
	Locations string `json:"locations"`
	Seed      string `json:"seed"`
}

type wireJobSourceResolution struct {
	jobSourceContext
	Unavailable bool `json:"unavailable"`
}

func (c *Client) resolveJobSource(ctx context.Context, source string) (jobSourceContext, error) {
	inv, err := c.scripts.Jobs(0, source)
	if err != nil {
		return jobSourceContext{}, err
	}
	rendered := source == "profile_match"
	wire, err := readWire[wireJobSourceResolution](ctx, c, rendered, "resolve_job_source", "/jobs/", inv.Expression)
	if err != nil {
		return jobSourceContext{}, err
	}
	if wire.Unavailable {
		return jobSourceContext{}, ErrJobSourceUnavailable
	}
	if !wire.jobSourceContext.validFor(source) {
		err := contractError("resolve_job_source", "invalid recommendation source context", nil)
		c.logContractError("resolve_job_source", err)
		return jobSourceContext{}, err
	}
	return wire.jobSourceContext, nil
}

func (link jobSourceContext) validFor(source string) bool {
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

func (link jobSourceContext) encode() (string, error) {
	raw, err := json.Marshal(link)
	if err != nil {
		return "", contractError("job_source_context", "encode source context", err)
	}
	return string(raw), nil
}

func decodeSourceContext(raw, source string) (jobSourceContext, error) {
	var link jobSourceContext
	if err := json.Unmarshal([]byte(raw), &link); err != nil || !link.validFor(source) {
		return jobSourceContext{}, errors.New("invalid recommendation source context")
	}
	return link, nil
}

func (link jobSourceContext) searchPath(page int) string {
	args := url.Values{
		"keywords": {link.Keywords},
		"origin":   {link.Origin},
		"geoId":    {link.Locations},
	}
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
