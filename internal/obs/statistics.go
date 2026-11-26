package obs

import (
	"github.com/andreykaipov/goobs"
	"github.com/andreykaipov/goobs/api/requests/general"
	"github.com/andreykaipov/goobs/api/requests/stream"
)

// StatisticsEpoch identifies the current auxiliary connection.
func (a *Aux) StatisticsEpoch() any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cur
}
func (a *Aux) GetStreamStatus() (*stream.GetStreamStatusResponse, error) {
	return auxCall(a, "GetStreamStatus", func(c *goobs.Client) (*stream.GetStreamStatusResponse, error) { return c.Stream.GetStreamStatus() })
}
func (a *Aux) GetStats() (*general.GetStatsResponse, error) {
	return auxCall(a, "GetStats", func(c *goobs.Client) (*general.GetStatsResponse, error) { return c.General.GetStats() })
}
