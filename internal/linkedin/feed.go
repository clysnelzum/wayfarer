package linkedin

import (
	"context"
	"fmt"
)

// Feed reads cards from the signed-in user's LinkedIn feed. A limit of 0 reads
// the initially rendered batch without scrolling; a positive limit up to
// MaxFeedLimit enables bounded scrolling and loading.
func (c *Client) Feed(ctx context.Context, limit int) (FeedResults, error) {
	if limit < 0 || limit > MaxFeedLimit {
		return FeedResults{}, fmt.Errorf("%w: feed limit must be between 0 and %d", ErrInvalidInput, MaxFeedLimit)
	}
	if err := c.ensureReady(); err != nil {
		return FeedResults{}, err
	}
	inv, err := c.scripts.Feed(limit)
	if err != nil {
		return FeedResults{}, err
	}
	wire, err := readWire[wireFeedResults](ctx, c, false, "feed", "/feed/foryou/", inv.Expression)
	if err != nil {
		return FeedResults{}, err
	}
	return wire.toDomain(limit)
}
