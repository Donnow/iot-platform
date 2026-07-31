package devicesim

import (
	"context"
	"errors"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const pahoTokenTimeout = 15 * time.Second

func NewPahoFactory() ClientFactory {
	return func(options MQTTOptions) (Client, error) {
		if options.BrokerURL == "" {
			return nil, errors.New("broker URL is required")
		}
		lost := make(chan error, 1)
		clientOptions := mqtt.NewClientOptions()
		clientOptions.AddBroker(options.BrokerURL)
		clientOptions.SetClientID(options.ClientID)
		clientOptions.SetUsername(options.Username)
		clientOptions.SetPassword(options.Password)
		clientOptions.SetCleanSession(false)
		clientOptions.SetAutoReconnect(false)
		clientOptions.SetWill(options.WillTopic, string(options.WillPayload), options.WillQoS, options.WillRetained)
		clientOptions.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			select {
			case lost <- err:
			default:
			}
		})
		return &pahoClient{client: mqtt.NewClient(clientOptions), lost: lost}, nil
	}
}

type pahoClient struct {
	client mqtt.Client
	lost   chan error
}

func (c *pahoClient) Connect(ctx context.Context) error {
	token := c.client.Connect()
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *pahoClient) Subscribe(ctx context.Context, topic string, qos byte, handler MessageHandler) error {
	token := c.client.Subscribe(topic, qos, func(_ mqtt.Client, message mqtt.Message) {
		handler(Message{Topic: message.Topic(), Payload: append([]byte(nil), message.Payload()...)})
	})
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *pahoClient) Publish(ctx context.Context, topic string, qos byte, retained bool, payload []byte) error {
	token := c.client.Publish(topic, qos, retained, payload)
	if err := waitToken(ctx, token); err != nil {
		return err
	}
	return token.Error()
}

func (c *pahoClient) Lost() <-chan error {
	return c.lost
}

func (c *pahoClient) Disconnect(_ context.Context) error {
	c.client.Disconnect(250)
	return nil
}

func waitToken(ctx context.Context, token mqtt.Token) error {
	if token.WaitTimeout(pahoTokenTimeout) {
		return nil
	}
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return errors.New("MQTT operation timed out")
}
