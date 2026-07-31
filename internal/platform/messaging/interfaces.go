package messaging

import "context"

const QoSAtLeastOnce byte = 1

type Publisher interface {
	Publish(context.Context, string, byte, bool, []byte) error
}
