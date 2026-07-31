package messaging

import "fmt"

func DeviceTopic(productKey, deviceID, suffix string) string {
	return fmt.Sprintf("devices/%s/%s/%s", productKey, deviceID, suffix)
}
