// Package reading turns MQTT messages into Readings: one value of one metric
// of one device at one time, matching a row of the readings table (ADR-0001).
package reading

import "time"

// Reading is one measurement, ready to be stored.
type Reading struct {
	Time   time.Time
	Device string
	Metric string
	Value  float64
}
