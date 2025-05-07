package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Sudo-Ivan/jacked-api/jacked"
)

// setSecurityHeaders sets appropriate security headers.
func setSecurityHeaders(w http.ResponseWriter) {
	csp := "default-src 'self'; " +
		"script-src 'self' https://cdn.jsdelivr.net; " +
		"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com; " +
		"font-src 'self' https://fonts.gstatic.com https://fonts.googleapis.com; " +
		"img-src 'self' data: https://*.tile.openstreetmap.org https://tile.openstreetmap.org; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'; " +
		"block-all-mixed-content; " +
		"upgrade-insecure-requests;"

	w.Header().Set("Content-Security-Policy", csp)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-XSS-Protection", "1; mode=block")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
}

var (
	alertCriteria   []AlertCriteria
	triggeredAlerts []Alert
	mu              sync.Mutex
	hub             *Hub
)

// Client represents a single SSE client connection.
type Client struct {
	ID   string
	Send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

func newHub() *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("Client registered: %s", client.ID)
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				log.Printf("Client unregistered: %s", client.ID)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					log.Printf("Client %s send buffer full or disconnected. Unregistering.", client.ID)
					delete(h.clients, client)
					close(client.Send)
				}
			}
		}
	}
}

func main() {
	hub = newHub()
	go hub.run()

	customJackedConfig := jacked.DefaultConfig()

	customJackedConfig.WriteTimeout = 5 * time.Minute
	customJackedConfig.IdleTimeout = 30 * time.Minute

	app := jacked.NewWithConfig(customJackedConfig)

	alertCriteria = append(alertCriteria, AlertCriteria{Callsign: "TARGET1"})
	alertCriteria = append(alertCriteria, AlertCriteria{ICAO: "AABBCC"})

	staticDir := "./public"

	app.GET("/", func(c *jacked.Context) error {
		setSecurityHeaders(c.Response)
		start := time.Now()
		filePath := staticDir + "/index.html"
		log.Printf("Serving file for /: %s", filePath)
		http.ServeFile(c.Response, c.Request, filePath)
		log.Printf("Served file for / in %v", time.Since(start))
		return nil
	})

	staticPath := "/static/*filepath"
	app.GET(staticPath, func(c *jacked.Context) error {
		start := time.Now()
		log.Printf("Serving static file for path: %s", c.Request.URL.Path)
		fs := http.StripPrefix("/static/", http.FileServer(http.Dir(staticDir)))
		fs.ServeHTTP(c.Response, c.Request)
		log.Printf("Served static file for path %s in %v", c.Request.URL.Path, time.Since(start))
		return nil
	})

	app.POST("/api/aircraft", func(c *jacked.Context) error {
		var aircraft Aircraft
		if err := json.NewDecoder(c.Request.Body).Decode(&aircraft); err != nil {
			log.Printf("Error decoding aircraft data: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid aircraft data"})
		}
		defer c.Request.Body.Close()

		aircraft.Timestamp = time.Now()
		log.Printf("Received aircraft data: %+v", aircraft)

		mu.Lock()
		aircraftUpdateJSON, err := json.Marshal(aircraft)
		if err != nil {
			log.Printf("Error marshalling aircraft data for SSE update: %v", err)
		} else {
			hub.broadcast <- []byte("event: aircraftUpdate\ndata: " + string(aircraftUpdateJSON) + "\n\n")
		}

		for _, criterion := range alertCriteria {
			match := false
			if criterion.ICAO != "" && criterion.ICAO == aircraft.ICAO {
				match = true
			}
			if criterion.Callsign != "" && criterion.Callsign == aircraft.Callsign {
				match = true
			}

			if match {
				alert := Alert{
					Aircraft:  aircraft,
					Message:   "Monitored aircraft detected: " + aircraft.Callsign + " (" + aircraft.ICAO + ")",
					Criteria:  criterion,
					Timestamp: time.Now(),
				}
				triggeredAlerts = append(triggeredAlerts, alert)
				log.Printf("ALERT: %+v", alert)

				alertJSON, err := json.Marshal(alert)
				if err != nil {
					log.Printf("Error marshalling alert for SSE: %v", err)
				} else {
					hub.broadcast <- []byte("event: alert\ndata: " + string(alertJSON) + "\n\n")
				}
			}
		}
		mu.Unlock()

		return c.JSON(http.StatusOK, map[string]string{"status": "received"})
	})

	app.GET("/api/alerts", func(c *jacked.Context) error {
		mu.Lock()
		defer mu.Unlock()
		alertsToReturn := make([]Alert, len(triggeredAlerts))
		copy(alertsToReturn, triggeredAlerts)
		return c.JSON(http.StatusOK, alertsToReturn)
	})

	app.POST("/api/alert-criteria", func(c *jacked.Context) error {
		var criterion AlertCriteria
		if err := json.NewDecoder(c.Request.Body).Decode(&criterion); err != nil {
			log.Printf("Error decoding alert criteria: %v", err)
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid criteria data"})
		}
		defer c.Request.Body.Close()

		mu.Lock()
		alertCriteria = append(alertCriteria, criterion)
		mu.Unlock()

		log.Printf("Added new alert criterion: %+v", criterion)
		return c.JSON(http.StatusCreated, criterion)
	})

	app.GET("/api/events", func(c *jacked.Context) error {
		c.Response.Header().Set("Content-Type", "text/event-stream")
		c.Response.Header().Set("Cache-Control", "no-cache")
		c.Response.Header().Set("Connection", "keep-alive")
		c.Response.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := c.Response.(http.Flusher)
		if !ok {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Streaming unsupported!"})
		}

		client := &Client{
			ID:   c.Request.RemoteAddr,
			Send: make(chan []byte, 256),
		}
		hub.register <- client

		defer func() {
			hub.unregister <- client
			log.Printf("SSE client %s connection closed (handler defer).", client.ID)
		}()

		log.Printf("SSE: Client %s entering send loop.", client.ID)
		for {
			select {
			case message, open := <-client.Send:
				if !open {
					log.Printf("SSE: Client %s send channel closed. Exiting loop.", client.ID)
					return nil
				}
				_, err := c.Response.Write(message)
				if err != nil {
					log.Printf("SSE: Error writing to client %s: %v. Exiting loop.", client.ID, err)
					return nil
				}
				flusher.Flush()
			case <-c.Request.Context().Done():
				log.Printf("SSE: Client %s context done. Exiting loop.", client.ID)
				return nil
			}
		}
	})

	listenAddr := ":8080"
	log.Printf("Aircraft Alert Server starting on %s (with custom timeouts for SSE)", listenAddr)

	go func() {
		if err := app.ListenAndServe(listenAddr); err != nil {
			log.Fatal(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	log.Println("Server exiting")
}
