package reading

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	gatewayPrefix = "heatpump-monitor/gateway/"
	boosterPrefix = "heatbooster/"

	// maxFutureSkew is how far a Gateway ts may be ahead of receivedAt.
	maxFutureSkew = 5 * time.Minute
)

var (
	// ErrFutureTimestamp: Gateway ts is more than 5 minutes ahead of receipt time.
	ErrFutureTimestamp = errors.New("timestamp too far in the future")
	// ErrUnknownDevice: Gateway topic names a device we do not know.
	ErrUnknownDevice = errors.New("unknown gateway device")
	// ErrInvalidPayload: payload is not in the expected shape.
	ErrInvalidPayload = errors.New("invalid payload")
)

var gatewayDevices = map[string]bool{
	"heat_pump":         true,
	"electricity_meter": true,
	"heat_meter":        true,
}

// boosterDevices maps the lower-cased Heat Booster name in the topic to a device.
// Boosters not listed here are ignored.
var boosterDevices = map[string]string{
	"heatbooster-links":  "heat_booster_links",
	"heatbooster-rechts": "heat_booster_rechts",
}

// boosterFields is the allowlist of Heat Booster topic fields and their metric
// names. Other fields (ssid, temp-delta-io, ...) are ignored.
var boosterFields = map[string]string{
	"temp-inlet":    "flow_temp",
	"temp-outlet":   "return_temp",
	"temp-ambient":  "ambient_temp",
	"fan-speed":     "fan_speed",
	"fan-boostmode": "fan_boost",
	"rssi":          "rssi",
}

// Decode turns one MQTT message into Readings. Messages that are not for us
// (unrelated topic, retained, non-allowlisted Heat Booster field, unknown
// Heat Booster name) return (nil, nil). On error no Readings are returned.
func Decode(topic string, payload []byte, retained bool, receivedAt time.Time) ([]Reading, error) {
	if retained {
		return nil, nil
	}
	var (
		rs  []Reading
		err error
	)
	switch {
	case strings.HasPrefix(topic, gatewayPrefix):
		rs, err = decodeGateway(strings.TrimPrefix(topic, gatewayPrefix), payload, receivedAt)
	case strings.HasPrefix(topic, boosterPrefix):
		rs, err = decodeBooster(strings.TrimPrefix(topic, boosterPrefix), payload, receivedAt)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode %q: %w", topic, err)
	}
	return rs, nil
}

type gatewayMessage struct {
	TS      string          `json:"ts"`
	Metrics json.RawMessage `json:"metrics"`
}

func decodeGateway(device string, payload []byte, receivedAt time.Time) ([]Reading, error) {
	if !gatewayDevices[device] {
		return nil, fmt.Errorf("%w: %q", ErrUnknownDevice, device)
	}
	var msg gatewayMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPayload, err)
	}
	at, err := gatewayTime(msg.TS, receivedAt)
	if err != nil {
		return nil, err
	}
	var metrics map[string]any
	if err := json.Unmarshal(msg.Metrics, &metrics); err != nil || metrics == nil {
		return nil, fmt.Errorf("%w: metrics must be an object", ErrInvalidPayload)
	}
	names := make([]string, 0, len(metrics))
	for name := range metrics {
		names = append(names, name)
	}
	sort.Strings(names)
	var out []Reading
	for _, name := range names {
		v, ok := metrics[name].(float64)
		if !ok {
			return nil, fmt.Errorf("%w: metric %q is not a number", ErrInvalidPayload, name)
		}
		out = append(out, Reading{Time: at, Device: device, Metric: name, Value: v})
	}
	return out, nil
}

// gatewayTime returns the message time in UTC: ts if present, else receivedAt.
func gatewayTime(ts string, receivedAt time.Time) (time.Time, error) {
	if ts == "" {
		return receivedAt.UTC(), nil
	}
	at, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: ts: %v", ErrInvalidPayload, err)
	}
	if at.After(receivedAt.Add(maxFutureSkew)) {
		return time.Time{}, fmt.Errorf("%w: %s", ErrFutureTimestamp, ts)
	}
	return at.UTC(), nil
}

// decodeBooster handles the part of the topic after "heatbooster/": <name>/<field>.
func decodeBooster(rest string, payload []byte, receivedAt time.Time) ([]Reading, error) {
	name, field, ok := strings.Cut(rest, "/")
	if !ok {
		return nil, nil
	}
	device, ok := boosterDevices[strings.ToLower(name)]
	if !ok {
		return nil, nil
	}
	metric, ok := boosterFields[field]
	if !ok {
		return nil, nil
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(payload)), 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("%w: %s value %q is not a number", ErrInvalidPayload, field, payload)
	}
	return []Reading{{Time: receivedAt.UTC(), Device: device, Metric: metric, Value: v}}, nil
}
