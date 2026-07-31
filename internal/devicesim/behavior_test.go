package devicesim

import (
	"math"
	"strings"
	"testing"
)

func TestTemperatureBehaviorStaysInRange(t *testing.T) {
	behavior := NewTemperatureBehavior(100)
	values, events := behavior.Tick(&fixedRandom{values: []float64{1, 1}})
	if len(events) != 0 {
		t.Fatalf("unexpected events: %#v", events)
	}
	if values["temperature"] != float64(45) || values["humidity"] != float64(90) {
		t.Fatalf("values = %#v, want clamped values", values)
	}
	values, _ = behavior.Tick(nil)
	if math.IsNaN(values["temperature"].(float64)) || math.IsInf(values["temperature"].(float64), 0) {
		t.Fatal("temperature must remain finite")
	}
}

func TestSmokeBehaviorEmitsOneEventOnThresholdCrossing(t *testing.T) {
	behavior := NewSmokeBehavior(100, 50)
	random := &fixedRandom{values: []float64{0, 1, 1}}
	if _, events := behavior.Tick(random); len(events) != 0 {
		t.Fatal("below threshold must not alert")
	}
	if _, events := behavior.Tick(random); len(events) != 1 || events[0].EventType != "alarm" {
		t.Fatalf("crossing events = %#v", events)
	}
	if _, events := behavior.Tick(random); len(events) != 0 {
		t.Fatal("continued threshold violation must not duplicate alert")
	}
}

func TestDoorBehaviorHandlesCommands(t *testing.T) {
	behavior := NewDoorBehavior()
	reply, values := behavior.HandleCommand(Command{CommandID: "c1", Method: "open"})
	if reply.Code != 0 || values["door_status"] != "open" {
		t.Fatalf("open reply = %#v, values = %#v", reply, values)
	}
	reply, values = behavior.HandleCommand(Command{CommandID: "c2", Method: "invalid"})
	if reply.Code == 0 || values != nil {
		t.Fatalf("invalid reply = %#v, values = %#v", reply, values)
	}
}

func TestAirConditionerRejectsPartialInvalidCommand(t *testing.T) {
	behavior := NewAirConditionerBehavior(1)
	before := behavior.values()
	reply, values := behavior.HandleCommand(Command{
		CommandID: "c1",
		Method:    "setTemp",
		Params:    map[string]any{"target": float64(26), "mode": "invalid"},
	})
	if reply.Code == 0 || values != nil {
		t.Fatalf("invalid setTemp reply = %#v, values = %#v", reply, values)
	}
	if after := behavior.values(); after["target_temp"] != before["target_temp"] || after["mode"] != before["mode"] {
		t.Fatalf("state changed after invalid command: before=%#v after=%#v", before, after)
	}
}

func TestAirConditionerConvergesToTarget(t *testing.T) {
	behavior := NewAirConditionerBehavior(0)
	reply, _ := behavior.HandleCommand(Command{
		CommandID: "c1",
		Method:    "setTemp",
		Params:    map[string]any{"target": float64(26), "mode": "cooling"},
	})
	if reply.Code != 0 {
		t.Fatalf("setTemp reply = %#v", reply)
	}
	for i := 0; i < 2; i++ {
		behavior.Tick(nil)
	}
	values, _ := behavior.Tick(nil)
	if values["current_temp"] != float64(26) || values["target_temp"] != float64(26) {
		t.Fatalf("values = %#v", values)
	}
}

func TestOTAValidation(t *testing.T) {
	valid := OTARequest{Version: "1.2.0", URL: "https://example.test/fw.bin", MD5: strings.Repeat("a", 32)}
	events := NewDoorBehavior().HandleOTA(valid)
	if len(events) != 3 || events[2].Data["stage"] != "success" {
		t.Fatalf("valid OTA events = %#v", events)
	}
	invalid := valid
	invalid.MD5 = strings.Repeat("z", 32)
	events = NewDoorBehavior().HandleOTA(invalid)
	if len(events) != 1 || events[0].Data["stage"] != "failed" {
		t.Fatalf("invalid OTA events = %#v", events)
	}
}
