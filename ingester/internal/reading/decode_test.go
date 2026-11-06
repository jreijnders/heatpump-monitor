package reading

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

var now = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func TestDecodeGateway(t *testing.T) {
	ts := time.Date(2026, 11, 6, 11, 59, 50, 0, time.UTC)
	tests := []struct {
		name    string
		topic   string
		payload string
		want    []Reading
		wantErr error // non-nil: any error; ErrFutureTimestamp checked via errors.Is
		anyErr  bool
	}{
		{
			name:    "three metrics use payload ts, sorted by metric",
			topic:   "heatpump-monitor/gateway/heat_pump",
			payload: `{"ts":"2026-11-06T11:59:50Z","metrics":{"flow_temp":35.5,"compressor_on":1,"outdoor_temp":-2}}`,
			want: []Reading{
				{ts, "heat_pump", "compressor_on", 1},
				{ts, "heat_pump", "flow_temp", 35.5},
				{ts, "heat_pump", "outdoor_temp", -2},
			},
		},
		{
			name:    "ts with offset normalised to UTC",
			topic:   "heatpump-monitor/gateway/electricity_meter",
			payload: `{"ts":"2026-11-06T12:59:50+01:00","metrics":{"power":100}}`,
			want:    []Reading{{ts, "electricity_meter", "power", 100}},
		},
		{
			name:    "heat_meter device accepted",
			topic:   "heatpump-monitor/gateway/heat_meter",
			payload: `{"ts":"2026-11-06T11:59:50Z","metrics":{"energy":5}}`,
			want:    []Reading{{ts, "heat_meter", "energy", 5}},
		},
		{
			name:    "missing ts uses receivedAt",
			topic:   "heatpump-monitor/gateway/heat_pump",
			payload: `{"metrics":{"flow_temp":30}}`,
			want:    []Reading{{now, "heat_pump", "flow_temp", 30}},
		},
		{
			name:    "ts 4 min in future accepted",
			topic:   "heatpump-monitor/gateway/heat_pump",
			payload: `{"ts":"2026-11-06T12:04:00Z","metrics":{"a":1}}`,
			want:    []Reading{{now.Add(4 * time.Minute), "heat_pump", "a", 1}},
		},
		{
			name:    "ts 10 min in future rejected",
			topic:   "heatpump-monitor/gateway/heat_pump",
			payload: `{"ts":"2026-11-06T12:10:00Z","metrics":{"a":1}}`,
			wantErr: ErrFutureTimestamp,
		},
		{name: "malformed json", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":`, anyErr: true},
		{name: "metrics not an object", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":[1,2]}`, anyErr: true},
		{name: "metrics missing", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"ts":"2026-11-06T11:59:50Z"}`, anyErr: true},
		{name: "bad ts", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"ts":"yesterday","metrics":{}}`, anyErr: true},
		{name: "string value, no partial", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":{"a":1,"b":"2"}}`, anyErr: true},
		{name: "bool value", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":{"a":true}}`, anyErr: true},
		{name: "null value", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":{"a":null}}`, anyErr: true},
		{name: "unknown device", topic: "heatpump-monitor/gateway/toaster", payload: `{"metrics":{"a":1}}`, anyErr: true},
		{name: "empty metrics", topic: "heatpump-monitor/gateway/heat_pump", payload: `{"metrics":{}}`, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.topic, []byte(tt.payload), false, now)
			checkResult(t, got, err, tt.want, tt.wantErr, tt.anyErr)
		})
	}
}

func TestDecodeHeatBooster(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		payload string
		want    []Reading
		anyErr  bool
	}{
		{"mixed-case name, inlet", "heatbooster/Heatbooster-rechts/temp-inlet", "41.28", []Reading{{now, "heat_booster_rechts", "flow_temp", 41.28}}, false},
		{"links outlet, whitespace trimmed", "heatbooster/heatbooster-links/temp-outlet", " 38.5\n", []Reading{{now, "heat_booster_links", "return_temp", 38.5}}, false},
		{"ambient", "heatbooster/heatbooster-links/temp-ambient", "7", []Reading{{now, "heat_booster_links", "ambient_temp", 7}}, false},
		{"fan speed", "heatbooster/heatbooster-links/fan-speed", "1200", []Reading{{now, "heat_booster_links", "fan_speed", 1200}}, false},
		{"fan boostmode", "heatbooster/heatbooster-links/fan-boostmode", "1", []Reading{{now, "heat_booster_links", "fan_boost", 1}}, false},
		{"rssi negative", "heatbooster/heatbooster-links/rssi", "-61", []Reading{{now, "heat_booster_links", "rssi", -61}}, false},
		{"ssid ignored", "heatbooster/heatbooster-links/ssid", "MyWifi", nil, false},
		{"temp-delta-io ignored", "heatbooster/heatbooster-links/temp-delta-io", "3.1", nil, false},
		{"unknown booster ignored", "heatbooster/heatbooster-garage/temp-inlet", "41", nil, false},
		{"non-numeric allowlisted", "heatbooster/heatbooster-links/temp-inlet", "abc", nil, true},
		{"empty allowlisted", "heatbooster/heatbooster-links/temp-inlet", "", nil, true},
		{"NaN rejected", "heatbooster/heatbooster-links/temp-inlet", "NaN", nil, true},
		{"non-numeric ignored field ok", "heatbooster/heatbooster-links/ssid", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.topic, []byte(tt.payload), false, now)
			checkResult(t, got, err, tt.want, nil, tt.anyErr)
		})
	}
}

func TestDecodeSkipped(t *testing.T) {
	tests := []struct{ name, topic, payload string }{
		{"retained gateway", "heatpump-monitor/gateway/heat_pump", `{"metrics":{"a":1}}`},
		{"retained booster", "heatbooster/heatbooster-links/temp-inlet", "41"},
		{"retained invalid gateway still skipped", "heatpump-monitor/gateway/toaster", `garbage`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.topic, []byte(tt.payload), true, now)
			checkResult(t, got, err, nil, nil, false)
		})
	}
	t.Run("unrelated topic", func(t *testing.T) {
		got, err := Decode("homeassistant/humidor/state", []byte(`{"a":1}`), false, now)
		checkResult(t, got, err, nil, nil, false)
	})
}

func checkResult(t *testing.T, got []Reading, err error, want []Reading, wantErr error, anyErr bool) {
	t.Helper()
	if wantErr != nil || anyErr {
		if err == nil {
			t.Fatalf("want error, got nil (readings %v)", got)
		}
		if wantErr != nil && !errors.Is(err, wantErr) {
			t.Fatalf("error %v does not wrap %v", err, wantErr)
		}
		if len(got) != 0 {
			t.Fatalf("want no readings on error, got %v", got)
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) || (len(want) > 0 && !reflect.DeepEqual(got, want)) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
