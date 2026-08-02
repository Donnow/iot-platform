package main

// 端到端遥测延迟探针：
// 哨兵设备发布一条遥测 -> 轮询平台查询 API 直到数据可见，
// 测量"发布 -> 平台消费 -> 物模型校验 -> TDengine 落库 -> 查询可见"的端到端延迟。
//
// 用法:
//   go run ./cmd/latencyprobe -token <jwt> [-creds ./creds-stress.json] \
//     [-product-key stress-product] [-samples 20] [-interval 3s]
//
// 说明: 查询 API 的 from 参数为 Unix 秒; 返回的 timestamp 与 payload ts 同源,
// 因此以"返回记录 timestamp >= 发布时刻"判定本次样本可见, 不依赖时钟同步。

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type credential struct {
	DeviceID     string `json:"device_id"`
	DeviceSecret string `json:"device_secret"`
}

type telemetryResponse struct {
	Items []struct {
		Timestamp string `json:"timestamp"`
	} `json:"items"`
}

func main() {
	api := flag.String("api", "http://localhost:8080", "platform API base URL")
	token := flag.String("token", os.Getenv("IOT_API_TOKEN"), "admin JWT")
	broker := flag.String("broker", "tcp://localhost:1883", "MQTT broker URL")
	credsFile := flag.String("creds", "./creds-stress.json", "device credentials JSON")
	productKey := flag.String("product-key", "stress-product", "product key")
	deviceID := flag.String("device-id", "", "sentinel device id (default: first credential)")
	samples := flag.Int("samples", 20, "number of samples")
	interval := flag.Duration("interval", 3*time.Second, "delay between publishes")
	timeout := flag.Duration("timeout", 10*time.Second, "per-sample visibility timeout")
	flag.Parse()

	if *token == "" {
		fmt.Fprintln(os.Stderr, "need -token or IOT_API_TOKEN")
		os.Exit(1)
	}
	creds := readCreds(*credsFile)
	id := *deviceID
	secret := ""
	if id == "" {
		id = creds[0].DeviceID
		secret = creds[0].DeviceSecret
	} else {
		for _, c := range creds {
			if c.DeviceID == id {
				secret = c.DeviceSecret
				break
			}
		}
	}
	if secret == "" {
		fmt.Fprintf(os.Stderr, "device %s not found in %s\n", id, *credsFile)
		os.Exit(1)
	}

	opts := mqtt.NewClientOptions().
		AddBroker(*broker).
		SetClientID(id).SetUsername(id).SetPassword(secret).
		SetConnectTimeout(10 * time.Second)
	client := mqtt.NewClient(opts)
	if tok := client.Connect(); !tok.WaitTimeout(10*time.Second) || tok.Error() != nil {
		fmt.Fprintln(os.Stderr, "MQTT connect failed:", tok.Error())
		os.Exit(1)
	}
	defer client.Disconnect(200)

	topic := "devices/" + *productKey + "/" + id + "/telemetry"
	latencies := make([]time.Duration, 0, *samples)

	for i := 0; i < *samples; i++ {
		t0 := time.Now()
		payload, _ := json.Marshal(map[string]any{
			"ts": t0.UnixMilli(),
			"values": map[string]any{
				"temperature": 15 + rand.Float64()*30,
				"humidity":    30 + rand.Float64()*60,
			},
		})
		tok := client.Publish(topic, 1, false, payload)
		tok.WaitTimeout(5 * time.Second)
		if tok.Error() != nil {
			fmt.Printf("sample %2d: publish error: %v\n", i+1, tok.Error())
			continue
		}

		lat := waitVisible(*api, *token, id, t0, *timeout)
		if lat < 0 {
			fmt.Printf("sample %2d: TIMEOUT (not visible within %v)\n", i+1, *timeout)
			continue
		}
		latencies = append(latencies, lat)
		fmt.Printf("sample %2d: %v\n", i+1, lat.Round(time.Millisecond))
		time.Sleep(*interval)
	}

	if len(latencies) == 0 {
		fmt.Fprintln(os.Stderr, "no successful samples")
		os.Exit(1)
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	pct := func(p float64) time.Duration {
		idx := int(float64(len(latencies)-1) * p)
		return latencies[idx]
	}
	fmt.Printf("\n=== %d samples: success=%d/%d ===\n", len(latencies), len(latencies), *samples)
	fmt.Printf("min=%v  p50=%v  p95=%v  p99=%v  max=%v\n",
		latencies[0].Round(time.Millisecond), pct(0.50).Round(time.Millisecond),
		pct(0.95).Round(time.Millisecond), pct(0.99).Round(time.Millisecond),
		latencies[len(latencies)-1].Round(time.Millisecond))
}

func waitVisible(api, token, deviceID string, t0 time.Time, timeout time.Duration) time.Duration {
	// TDengine stores the payload ts at millisecond precision (the probe
	// publishes t0.UnixMilli()), so comparisons must use a ms-truncated
	// reference; a full-precision t0 would always be strictly after the
	// stored value and every sample would time out.
	t0ms := t0.Truncate(time.Millisecond)
	deadline := time.Now().Add(timeout)
	url := fmt.Sprintf("%s/api/devices/%s/telemetry?metric=temperature&from=%d&limit=100",
		api, deviceID, t0.Unix()-5)
	for time.Now().Before(deadline) {
		body := get(url, token)
		if body != nil {
			var resp telemetryResponse
			if json.Unmarshal(body, &resp) == nil {
				for _, item := range resp.Items {
					ts, err := time.Parse(time.RFC3339Nano, item.Timestamp)
					if err == nil && !ts.Before(t0ms) {
						return time.Since(t0)
					}
				}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return -1
}

func get(url, token string) []byte {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return body
}

func readCreds(path string) []credential {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read creds:", err)
		os.Exit(1)
	}
	var creds []credential
	if err := json.Unmarshal(data, &creds); err != nil || len(creds) == 0 {
		fmt.Fprintln(os.Stderr, "invalid creds file:", err)
		os.Exit(1)
	}
	return creds
}
