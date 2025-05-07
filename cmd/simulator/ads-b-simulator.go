package main

import (
	"bytes"
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// Aircraft struct definition for the simulator.
// This is a standalone program, so it defines its own struct.
// Ideally, this would be shared from a common models package in a larger project.
type Aircraft struct {
	ICAO      string    `json:"icao"`
	Callsign  string    `json:"callsign"`
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lon"`
	Altitude  int       `json:"alt_baro"`
	Speed     float64   `json:"gs"`    // Ground speed in knots
	Track     float64   `json:"track"` // Track angle in degrees
	Timestamp time.Time `json:"timestamp"`
}

const serverURL = "http://localhost:8090/api/aircraft"
const tickIntervalSeconds = 5
const earthRadiusKm = 6371.0

// liveAircraft will store the current state of all simulated aircraft
var liveAircraft []Aircraft

func initializeAircraft() {
	liveAircraft = []Aircraft{
		{ICAO: "AABBCC", Callsign: "TARGET1", Latitude: 34.0522, Longitude: -118.2437, Altitude: 35000, Speed: 450, Track: 45},
		{ICAO: "DDEEFF", Callsign: "NORMALFLT", Latitude: 40.7128, Longitude: -74.0060, Altitude: 30000, Speed: 500, Track: 120},
		{ICAO: "112233", Callsign: "LOWFLYER", Latitude: 34.0000, Longitude: -118.0000, Altitude: 5000, Speed: 180, Track: 270},
		{ICAO: "TARGET2", Callsign: "SPECIALVIP", Latitude: 48.8566, Longitude: 2.3522, Altitude: 39000, Speed: 480, Track: 310},
		{ICAO: "FFFF01", Callsign: "CIRCLER", Latitude: 30.0, Longitude: -90.0, Altitude: 10000, Speed: 250, Track: 0},
		{ICAO: "FFFF02", Callsign: "EASTBOUND", Latitude: 39.8617, Longitude: -104.6731, Altitude: 28000, Speed: 400, Track: 90}, // Denver Intl
	}
}

// updateAircraftPosition calculates new lat/lon based on speed, track, and time interval
func updateAircraftPosition(ac *Aircraft, dt float64) {
	// Convert speed from knots to km/s
	// 1 knot = 0.514444 m/s = 0.000514444 km/s
	speedKmPerSec := ac.Speed * 0.000514444
	distanceKm := speedKmPerSec * dt

	trackRad := ac.Track * math.Pi / 180.0 // Convert track to radians

	currentLatRad := ac.Latitude * math.Pi / 180.0
	currentLonRad := ac.Longitude * math.Pi / 180.0

	newLatRad := math.Asin(math.Sin(currentLatRad)*math.Cos(distanceKm/earthRadiusKm) +
		math.Cos(currentLatRad)*math.Sin(distanceKm/earthRadiusKm)*math.Cos(trackRad))

	newLonRad := currentLonRad + math.Atan2(math.Sin(trackRad)*math.Sin(distanceKm/earthRadiusKm)*math.Cos(currentLatRad),
		math.Cos(distanceKm/earthRadiusKm)-math.Sin(currentLatRad)*math.Sin(newLatRad))

	ac.Latitude = newLatRad * 180.0 / math.Pi
	ac.Longitude = newLonRad * 180.0 / math.Pi

	// Boundary checks for latitude and longitude (wrap longitude, clamp latitude)
	if ac.Latitude > 90 {
		ac.Latitude = 90
	}
	if ac.Latitude < -90 {
		ac.Latitude = -90
	}
	ac.Longitude = math.Mod(ac.Longitude+180, 360) - 180 // Wrap longitude to [-180, 180]

	// Simulate slight changes in altitude, speed, and track
	ac.Altitude += (rand.Intn(10) - 5) * 10 // +/- 50 feet adjustment
	if ac.Altitude < 100 {
		ac.Altitude = 100
	} // Min altitude
	if ac.Altitude > 55000 {
		ac.Altitude = 55000
	} // Max altitude

	ac.Speed += float64(rand.Intn(11) - 5) // +/- 5 knots
	if ac.Speed < 80 {
		ac.Speed = 80
	} // Min speed
	if ac.Speed > 600 {
		ac.Speed = 600
	} // Max speed

	// For the aircraft named CIRCLER, make it turn continuously
	if ac.Callsign == "CIRCLER" {
		ac.Track += 5 // degrees per tick
	} else {
		ac.Track += float64(rand.Intn(7) - 3) // +/- 3 degrees change for others
	}
	ac.Track = math.Mod(ac.Track, 360)
	if ac.Track < 0 {
		ac.Track += 360
	}
}

func main() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	initializeAircraft()

	log.Println("Starting ADS-B Simulator with more realistic movement...")
	log.Printf("Sending data to: %s every %d seconds", serverURL, tickIntervalSeconds)

	ticker := time.NewTicker(time.Duration(tickIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Iterate over a copy if modifying, or directly if careful
		for i := range liveAircraft {
			ac := &liveAircraft[i] // Get a pointer to modify the actual aircraft in the slice

			updateAircraftPosition(ac, float64(tickIntervalSeconds))
			ac.Timestamp = time.Now()

			jsonData, err := json.Marshal(ac)
			if err != nil {
				log.Printf("Error marshalling aircraft data for %s: %v", ac.Callsign, err)
				continue
			}

			// Send data for this specific aircraft
			// Stagger sends slightly to avoid bursting, or send one aircraft per tick for fewer updates
			// For now, sending all updated aircraft each tick.
			go func(data []byte, callsign string) {
				resp, err := http.Post(serverURL, "application/json", bytes.NewBuffer(data))
				if err != nil {
					log.Printf("Error sending aircraft data for %s: %v", callsign, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					log.Printf("Server responded with status: %s for %s", resp.Status, callsign)
				} else {
					// log.Printf("Sent data for: %s (%s), Server OK", callsign, ac.ICAO) // Too noisy
				}
			}(jsonData, ac.Callsign)
		}
		log.Printf("%d aircraft positions updated and sent.", len(liveAircraft))
	}
}
