package devicesim

import "time"

type Ticker interface {
	Chan() <-chan time.Time
	Stop()
}

type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
	After(time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{ticker: time.NewTicker(interval)}
}

func (realClock) After(interval time.Duration) <-chan time.Time {
	return time.After(interval)
}

type realTicker struct {
	ticker *time.Ticker
}

func (t realTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t realTicker) Stop() {
	t.ticker.Stop()
}
