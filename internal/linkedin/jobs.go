package linkedin

import (
	"context"
	"fmt"
	"time"
)

// Jobs reads LinkedIn job recommendations, personalized recommendation-source
// searches, or paginated keyword searches according to query.
func (c *Client) Jobs(ctx context.Context, query JobQuery) (JobResults, error) {
	path, err := query.searchPath()
	if err != nil {
		return JobResults{}, err
	}
	if err := c.ensureReady(); err != nil {
		return JobResults{}, err
	}

	resolvedSourceCtx := query.SourceContext
	if query.Source != "" {
		var link jobSourceContext
		if query.SourceContext != "" {
			link, err = decodeSourceContext(query.SourceContext, query.Source)
			if err != nil {
				return JobResults{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
			}
		} else {
			resolveCtx, cancel := context.WithTimeout(ctx, 26*time.Second)
			link, err = c.resolveJobSource(resolveCtx, query.Source)
			cancel()
			if err != nil {
				return JobResults{}, err
			}
			resolvedSourceCtx, err = link.encode()
			if err != nil {
				return JobResults{}, err
			}
		}
		path = link.searchPath(query.Page)
	}

	inv, err := c.scripts.Jobs(query.RecommendationLimit, "")
	if err != nil {
		return JobResults{}, err
	}
	rendered := query.RecommendationLimit > 0
	wire, err := readWire[wireJobsResponse](ctx, c, rendered, "jobs", path, inv.Expression)
	if err != nil {
		return JobResults{}, err
	}
	return wire.toDomain(query, path, resolvedSourceCtx)
}

// Job reads full posting details for a single numeric LinkedIn job ID.
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	if !ValidJobID(id) {
		return Job{}, fmt.Errorf("%w: invalid job id", ErrInvalidInput)
	}
	if err := c.ensureReady(); err != nil {
		return Job{}, err
	}
	inv, err := c.scripts.JobDetail()
	if err != nil {
		return Job{}, err
	}
	wire, err := readWire[wireJobResponse](ctx, c, false, "job", "/jobs/view/"+id+"/", inv.Expression)
	if err != nil {
		return Job{}, err
	}
	return wire.toDomain(id, c.baseURL)
}
