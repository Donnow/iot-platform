package devicesim

import (
	"context"
	"errors"
	"sync"
	"time"
)

type fixedRandom struct {
	values []float64
	index  int
}

func (r *fixedRandom) Float64() float64 {
	if len(r.values) == 0 {
		return 0.5
	}
	value := r.values[r.index%len(r.values)]
	r.index++
	return value
}

type fakeTicker struct {
	ch      chan time.Time
	mu      sync.Mutex
	stopped bool
}

func newFakeTicker() *fakeTicker {
	return &fakeTicker{ch: make(chan time.Time, 8)}
}

func (t *fakeTicker) Chan() <-chan time.Time {
	return t.ch
}

func (t *fakeTicker) Stop() {
	t.mu.Lock()
	t.stopped = true
	t.mu.Unlock()
}

type fakeClock struct {
	now    time.Time
	ticker *fakeTicker
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.UnixMilli(1722000000000), ticker: newFakeTicker()}
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

func (c *fakeClock) NewTicker(time.Duration) Ticker {
	c.ticker = newFakeTicker()
	return c.ticker
}

func (c *fakeClock) After(interval time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- c.now.Add(interval)
	return ch
}

type publishedMessage struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

type fakeSubscription struct {
	Topic   string
	QoS     byte
	Handler MessageHandler
}

type fakeClient struct {
	mu           sync.Mutex
	connected    bool
	disconnected bool
	subs         []fakeSubscription
	published    []publishedMessage
	lost         chan error
}

func newFakeClient() *fakeClient {
	return &fakeClient{lost: make(chan error, 2)}
}

func (c *fakeClient) Connect(ctx context.Context) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Subscribe(ctx context.Context, topic string, qos byte, handler MessageHandler) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.subs = append(c.subs, fakeSubscription{Topic: topic, QoS: qos, Handler: handler})
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	c.mu.Lock()
	c.published = append(c.published, publishedMessage{
		Topic:    topic,
		QoS:      qos,
		Retained: retained,
		Payload:  append([]byte(nil), payload...),
	})
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Lost() <-chan error {
	return c.lost
}

func (c *fakeClient) Disconnect(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	c.mu.Lock()
	c.disconnected = true
	c.connected = false
	c.mu.Unlock()
	return nil
}

func (c *fakeClient) Subscriptions() []fakeSubscription {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]fakeSubscription(nil), c.subs...)
}

func (c *fakeClient) Published() []publishedMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	messages := make([]publishedMessage, len(c.published))
	copy(messages, c.published)
	for i := range messages {
		messages[i].Payload = append([]byte(nil), messages[i].Payload...)
	}
	return messages
}

func (c *fakeClient) Deliver(topic string, payload []byte) error {
	c.mu.Lock()
	var handler MessageHandler
	for _, subscription := range c.subs {
		if subscription.Topic == topic {
			handler = subscription.Handler
			break
		}
	}
	c.mu.Unlock()
	if handler == nil {
		return errors.New("no matching subscription")
	}
	handler(Message{Topic: topic, Payload: append([]byte(nil), payload...)})
	return nil
}

type fakeFactory struct {
	mu      sync.Mutex
	options []MQTTOptions
	clients []*fakeClient
}

func (f *fakeFactory) Create(options MQTTOptions) (Client, error) {
	client := newFakeClient()
	f.mu.Lock()
	f.options = append(f.options, options)
	f.clients = append(f.clients, client)
	f.mu.Unlock()
	return client, nil
}

func (f *fakeFactory) Clients() []*fakeClient {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*fakeClient(nil), f.clients...)
}

func (f *fakeFactory) Options() []MQTTOptions {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]MQTTOptions(nil), f.options...)
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
