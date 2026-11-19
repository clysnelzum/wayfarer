package linkedin

import (
	"context"
	"fmt"
	"regexp"
)

var slugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]{0,99}$`)

// ValidSlug reports whether slug is a valid LinkedIn personal or company URL slug.
func ValidSlug(slug string) bool {
	return slugPattern.MatchString(slug)
}

// Profile reads a rendered personal LinkedIn profile for slug.
func (c *Client) Profile(ctx context.Context, slug string) (Profile, error) {
	if !ValidSlug(slug) {
		return Profile{}, fmt.Errorf("%w: invalid profile slug", ErrInvalidInput)
	}
	if err := c.ensureReady(); err != nil {
		return Profile{}, err
	}
	inv, err := c.scripts.Profile()
	if err != nil {
		return Profile{}, err
	}
	wire, err := readWire[wireProfileResponse](ctx, c, true, "profile", "/in/"+slug+"/", inv.Expression)
	if err != nil {
		return Profile{}, err
	}
	return wire.toDomain(slug, c.baseURL)
}

// Company reads a LinkedIn organization's About page and sidebar details for slug.
func (c *Client) Company(ctx context.Context, slug string) (Company, error) {
	if !ValidSlug(slug) {
		return Company{}, fmt.Errorf("%w: invalid company slug", ErrInvalidInput)
	}
	if err := c.ensureReady(); err != nil {
		return Company{}, err
	}
	inv, err := c.scripts.Company()
	if err != nil {
		return Company{}, err
	}
	wire, err := readWire[wireCompanyResponse](ctx, c, false, "company", "/company/"+slug+"/about/", inv.Expression)
	if err != nil {
		return Company{}, err
	}
	return wire.toDomain(slug, c.baseURL)
}
