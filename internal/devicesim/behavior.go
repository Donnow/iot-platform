package devicesim

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
)

type Behavior interface {
	Tick(Random) (map[string]any, []Event)
	HandleCommand(Command) (CommandReply, map[string]any)
	HandleShadow(map[string]any) (map[string]any, error)
	HandleOTA(OTARequest) []Event
}

type TemperatureBehavior struct {
	fluctuation float64
	temperature float64
	humidity    float64
	last        map[string]any
}

func NewTemperatureBehavior(fluctuation float64) *TemperatureBehavior {
	return &TemperatureBehavior{
		fluctuation: fluctuation,
		temperature: 24,
		humidity:    60,
		last:        map[string]any{"temperature": float64(24), "humidity": float64(60)},
	}
}

func (b *TemperatureBehavior) Tick(random Random) (map[string]any, []Event) {
	b.temperature = clamp(b.temperature+behaviorDelta(random, b.fluctuation), 15, 45)
	b.humidity = clamp(b.humidity+behaviorDelta(random, b.fluctuation), 30, 90)
	b.last = map[string]any{
		"temperature": b.temperature,
		"humidity":    b.humidity,
	}
	return cloneMap(b.last), nil
}

func (b *TemperatureBehavior) HandleCommand(command Command) (CommandReply, map[string]any) {
	return failedCommand(command.CommandID, "unsupported command for temperature sensor"), nil
}

func (b *TemperatureBehavior) HandleShadow(_ map[string]any) (map[string]any, error) {
	return cloneMap(b.last), nil
}

func (b *TemperatureBehavior) HandleOTA(request OTARequest) []Event {
	return otaEvents(request)
}

type SmokeBehavior struct {
	fluctuation float64
	threshold   float64
	level       float64
	lastAbove   bool
	last        map[string]any
}

func NewSmokeBehavior(fluctuation, threshold float64) *SmokeBehavior {
	return &SmokeBehavior{
		fluctuation: fluctuation,
		threshold:   threshold,
		last:        map[string]any{"smoke_level": float64(0)},
	}
}

func (b *SmokeBehavior) Tick(random Random) (map[string]any, []Event) {
	b.level = clamp(b.level+behaviorDelta(random, b.fluctuation), 0, 100)
	b.last = map[string]any{"smoke_level": b.level}
	above := b.level > b.threshold
	var events []Event
	if above && !b.lastAbove {
		events = []Event{{
			EventType: "alarm",
			Data: map[string]any{
				"metric":    "smoke_level",
				"value":     b.level,
				"threshold": b.threshold,
			},
		}}
	}
	b.lastAbove = above
	return cloneMap(b.last), events
}

func (b *SmokeBehavior) HandleCommand(command Command) (CommandReply, map[string]any) {
	return failedCommand(command.CommandID, "unsupported command for smoke detector"), nil
}

func (b *SmokeBehavior) HandleShadow(_ map[string]any) (map[string]any, error) {
	return cloneMap(b.last), nil
}

func (b *SmokeBehavior) HandleOTA(request OTARequest) []Event {
	return otaEvents(request)
}

type DoorBehavior struct {
	status string
	last   map[string]any
}

func NewDoorBehavior() *DoorBehavior {
	return &DoorBehavior{
		status: "closed",
		last:   map[string]any{"door_status": "closed"},
	}
}

func (b *DoorBehavior) Tick(_ Random) (map[string]any, []Event) {
	return cloneMap(b.last), nil
}

func (b *DoorBehavior) HandleCommand(command Command) (CommandReply, map[string]any) {
	if command.Method != "open" && command.Method != "close" {
		return failedCommand(command.CommandID, "unsupported door command"), nil
	}
	if len(command.Params) != 0 {
		return failedCommand(command.CommandID, "door command does not accept params"), nil
	}
	if command.Method == "open" {
		b.status = "open"
	} else {
		b.status = "closed"
	}
	b.last = map[string]any{"door_status": b.status}
	return successCommand(command.CommandID, command.Method+" accepted"), cloneMap(b.last)
}

func (b *DoorBehavior) HandleShadow(_ map[string]any) (map[string]any, error) {
	return cloneMap(b.last), nil
}

func (b *DoorBehavior) HandleOTA(request OTARequest) []Event {
	return otaEvents(request)
}

type AirConditionerBehavior struct {
	fluctuation float64
	currentTemp float64
	targetTemp  float64
	mode        string
	last        map[string]any
}

func NewAirConditionerBehavior(fluctuation float64) *AirConditionerBehavior {
	b := &AirConditionerBehavior{
		fluctuation: fluctuation,
		currentTemp: 24,
		targetTemp:  24,
		mode:        "off",
	}
	b.last = b.values()
	return b
}

func (b *AirConditionerBehavior) Tick(_ Random) (map[string]any, []Event) {
	step := 1.0
	if b.currentTemp < b.targetTemp {
		b.currentTemp = minFloat(b.currentTemp+step, b.targetTemp)
	} else if b.currentTemp > b.targetTemp {
		b.currentTemp = maxFloat(b.currentTemp-step, b.targetTemp)
	}
	b.last = b.values()
	return cloneMap(b.last), nil
}

func (b *AirConditionerBehavior) HandleCommand(command Command) (CommandReply, map[string]any) {
	if command.Method != "setTemp" {
		return failedCommand(command.CommandID, "unsupported air-conditioner command"), nil
	}
	target, mode, err := parseAirConditionerCommand(command.Params)
	if err != nil {
		return failedCommand(command.CommandID, err.Error()), nil
	}
	b.targetTemp = target
	b.mode = mode
	b.last = b.values()
	return successCommand(command.CommandID, "setTemp accepted"), cloneMap(b.last)
}

func (b *AirConditionerBehavior) HandleShadow(desired map[string]any) (map[string]any, error) {
	target := b.targetTemp
	mode := b.mode
	for name, value := range desired {
		switch name {
		case "targetTemp":
			parsed, ok := numberValue(value)
			if !ok || !validAirConditionerTemperature(parsed) {
				return nil, errors.New("targetTemp must be between 16 and 30")
			}
			target = parsed
		case "mode":
			parsed, ok := value.(string)
			if !ok || !validMode(parsed) {
				return nil, fmt.Errorf("unsupported mode %q", value)
			}
			mode = parsed
		default:
			return nil, fmt.Errorf("unsupported shadow field %q", name)
		}
	}
	b.targetTemp = target
	b.mode = mode
	b.last = b.values()
	return cloneMap(b.last), nil
}

func (b *AirConditionerBehavior) HandleOTA(request OTARequest) []Event {
	return otaEvents(request)
}

func (b *AirConditionerBehavior) values() map[string]any {
	return map[string]any{
		"current_temp": b.currentTemp,
		"target_temp":  b.targetTemp,
		"mode":         b.mode,
	}
}

func parseAirConditionerCommand(params map[string]any) (float64, string, error) {
	target, ok := numberParam(params, "target")
	if !ok || !validAirConditionerTemperature(target) {
		return 0, "", errors.New("target must be between 16 and 30")
	}
	mode, ok := stringParam(params, "mode")
	if !ok || !validMode(mode) {
		return 0, "", errors.New("mode must be cooling, heating, auto, or off")
	}
	for name := range params {
		if name != "target" && name != "mode" {
			return 0, "", fmt.Errorf("unsupported setTemp param %q", name)
		}
	}
	return target, mode, nil
}

func buildBehavior(config Config) (Behavior, error) {
	switch config.DeviceType {
	case DeviceTypeTemperature:
		return NewTemperatureBehavior(config.Fluctuation), nil
	case DeviceTypeSmoke:
		return NewSmokeBehavior(config.Fluctuation, config.SmokeThreshold), nil
	case DeviceTypeDoor:
		return NewDoorBehavior(), nil
	case DeviceTypeAirConditioner:
		return NewAirConditionerBehavior(config.Fluctuation), nil
	default:
		return nil, fmt.Errorf("unsupported device type %q", config.DeviceType)
	}
}

func successCommand(commandID, message string) CommandReply {
	return CommandReply{CommandID: commandID, Code: 0, Message: message}
}

func failedCommand(commandID, message string) CommandReply {
	return CommandReply{CommandID: commandID, Code: 1, Message: message}
}

func otaEvents(request OTARequest) []Event {
	if !validOTARequest(request) {
		return []Event{{
			EventType: "ota_progress",
			Data: map[string]any{
				"version":  request.Version,
				"stage":    "failed",
				"progress": 0,
				"message":  "version, url, and 32-character md5 are required",
			},
		}}
	}
	return []Event{
		{EventType: "ota_progress", Data: map[string]any{"version": request.Version, "stage": "downloading", "progress": 0}},
		{EventType: "ota_progress", Data: map[string]any{"version": request.Version, "stage": "installing", "progress": 50}},
		{EventType: "ota_progress", Data: map[string]any{"version": request.Version, "stage": "success", "progress": 100}},
	}
}

func validOTARequest(request OTARequest) bool {
	if strings.TrimSpace(request.Version) == "" || strings.TrimSpace(request.Version) != request.Version {
		return false
	}
	if strings.TrimSpace(request.MD5) != request.MD5 || len(request.MD5) != 32 {
		return false
	}
	if _, err := hex.DecodeString(request.MD5); err != nil {
		return false
	}
	u, err := url.Parse(request.URL)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func validMode(mode string) bool {
	switch mode {
	case "cooling", "heating", "auto", "off":
		return true
	default:
		return false
	}
}

func numberParam(params map[string]any, name string) (float64, bool) {
	value, ok := params[name]
	if !ok {
		return 0, false
	}
	return numberValue(value)
}

func numberValue(value any) (float64, bool) {
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case float32:
		number = float64(value)
	case int:
		number = float64(value)
	case int8:
		number = float64(value)
	case int16:
		number = float64(value)
	case int32:
		number = float64(value)
	case int64:
		number = float64(value)
	case uint:
		number = float64(value)
	case uint8:
		number = float64(value)
	case uint16:
		number = float64(value)
	case uint32:
		number = float64(value)
	case uint64:
		number = float64(value)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func stringParam(params map[string]any, name string) (string, bool) {
	value, ok := params[name].(string)
	return value, ok
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func behaviorDelta(random Random, fluctuation float64) float64 {
	if random == nil {
		return 0
	}
	return randomDelta(random, fluctuation)
}

func validAirConditionerTemperature(value float64) bool {
	return value >= 16 && value <= 30 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func cloneMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
