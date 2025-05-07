package main

import "time"

// Aircraft represents basic ADS-B data for an aircraft.
type Aircraft struct {
	ICAO      string    `json:"icao"`      // Unique ICAO 24-bit address
	Callsign  string    `json:"callsign"`  // Callsign (e.g., SWA123, N123AB)
	Latitude  float64   `json:"lat"`       // Latitude in degrees
	Longitude float64   `json:"lon"`       // Longitude in degrees
	Altitude  int       `json:"alt_baro"`  // Barometric altitude in feet
	Speed     float64   `json:"gs"`        // Ground speed in knots
	Track     float64   `json:"track"`     // Track angle in degrees (clockwise from true north)
	Timestamp time.Time `json:"timestamp"` // Timestamp of the data
}

// AlertCriteria defines the conditions for an alert.
// We can match on any field of the Aircraft struct.
type AlertCriteria struct {
	ICAO     string `json:"icao,omitempty"`
	Callsign string `json:"callsign,omitempty"`
	// Add other fields as needed, e.g., geographic zones
}

// Alert represents an alert triggered for a specific aircraft.
type Alert struct {
	Aircraft  Aircraft      `json:"aircraft"`
	Message   string        `json:"message"`
	Criteria  AlertCriteria `json:"criteria"` // The criteria that triggered this alert
	Timestamp time.Time     `json:"timestamp"`
}
