package linkedin

import (
	"context"
	_ "embed"
	"errors"
	"regexp"
	"strings"
)

//go:embed profile.js
var profileExpression string

//go:embed company.js
var companyExpression string

var (
	ErrProfileNotFound = errors.New("personal profile not found")
	ErrCompanyNotFound = errors.New("company profile not found")
	slugPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,99}$`)
)

func ValidSlug(slug string) bool { return slugPattern.MatchString(slug) }

type Profile struct {
	Slug     string `json:"slug"`
	URL      string `json:"url"`
	Name     string `json:"name"`
	Headline string `json:"headline,omitempty"`
	Location string `json:"location,omitempty"`
	About    string `json:"about,omitempty"`
}

type Company struct {
	Slug         string `json:"slug"`
	URL          string `json:"url"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	Website      string `json:"website,omitempty"`
	Industry     string `json:"industry,omitempty"`
	CompanySize  string `json:"company_size,omitempty"`
	Headquarters string `json:"headquarters,omitempty"`
	Founded      string `json:"founded,omitempty"`
	CompanyType  string `json:"company_type,omitempty"`
	Specialties  string `json:"specialties,omitempty"`
}

func (c Client) Profile(ctx context.Context, slug string) (Profile, error) {
	if !ValidSlug(slug) {
		return Profile{}, errors.New("invalid profile slug")
	}
	v, err := read[struct {
		Data     Profile `json:"profile"`
		NotFound bool    `json:"not_found"`
	}](ctx, c.Reader, "/in/"+slug+"/", profileExpression)
	if err != nil {
		return Profile{}, err
	}
	if v.NotFound {
		return Profile{}, ErrProfileNotFound
	}
	if !strings.EqualFold(v.Data.Slug, slug) || v.Data.Name == "" {
		return Profile{}, errors.New("browser result did not match the requested profile")
	}
	v.Data.URL = base + "/in/" + v.Data.Slug + "/"
	return v.Data, nil
}

func (c Client) Company(ctx context.Context, slug string) (Company, error) {
	if !ValidSlug(slug) {
		return Company{}, errors.New("invalid company slug")
	}
	v, err := read[struct {
		Data     Company `json:"company"`
		NotFound bool    `json:"not_found"`
	}](ctx, c.Reader, "/company/"+slug+"/about/", companyExpression)
	if err != nil {
		return Company{}, err
	}
	if v.NotFound {
		return Company{}, ErrCompanyNotFound
	}
	if !strings.EqualFold(v.Data.Slug, slug) || v.Data.Name == "" {
		return Company{}, errors.New("browser result did not match the requested company")
	}
	v.Data.URL = base + "/company/" + v.Data.Slug + "/"
	return v.Data, nil
}
