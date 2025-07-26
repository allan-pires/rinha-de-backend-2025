package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
)

type PaymentRequest struct {
	CorrelationID string  `json:"correlationId" binding:"required"`
	Amount        float64 `json:"amount" binding:"required"`
}

type PaymentProcessorRequest struct {
	CorrelationID string    `json:"correlationId"`
	Amount        float64   `json:"amount"`
	RequestedAt   time.Time `json:"requestedAt"`
}

type PaymentSummary struct {
	Default  ProcessorSummary `json:"default"`
	Fallback ProcessorSummary `json:"fallback"`
}

type ProcessorSummary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}

type HealthCheck struct {
	Failing         bool `json:"failing"`
	MinResponseTime int  `json:"minResponseTime"`
}

type CircuitBreaker struct {
	defaultURL    string
	fallbackURL   string
	client        *http.Client
	healthMutex   sync.RWMutex
	lastHealthCheck map[string]time.Time
	isDefaultHealthy bool
	isFallbackHealthy bool
}

type App struct {
	db              *sql.DB
	circuitBreaker  *CircuitBreaker
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	
	app := &App{}
	
	// Initialize database connection
	if err := app.initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer app.db.Close()
	
	// Initialize circuit breaker
	app.initCircuitBreaker()
	
	// Start health check routine
	go app.circuitBreaker.healthCheckRoutine()
	
	// Setup routes
	router := gin.New()
	router.Use(gin.Recovery())
	
	router.POST("/payments", app.handlePayments)
	router.GET("/payments-summary", app.handlePaymentsSummary)
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	
	log.Printf("Starting server on port %s", port)
	log.Fatal(router.Run(":" + port))
}

func (app *App) initDB() error {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)
	
	var err error
	app.db, err = sql.Open("postgres", psqlInfo)
	if err != nil {
		return err
	}
	
	// Configure connection pool
	app.db.SetMaxOpenConns(20)
	app.db.SetMaxIdleConns(5)
	app.db.SetConnMaxLifetime(time.Hour)
	
	return app.db.Ping()
}

func (app *App) initCircuitBreaker() {
	defaultURL := os.Getenv("DEFAULT_PROCESSOR_URL")
	fallbackURL := os.Getenv("FALLBACK_PROCESSOR_URL")
	
	if defaultURL == "" {
		defaultURL = "http://payment-processor-default:8080"
	}
	if fallbackURL == "" {
		fallbackURL = "http://payment-processor-fallback:8080"
	}
	
	app.circuitBreaker = &CircuitBreaker{
		defaultURL:        defaultURL,
		fallbackURL:       fallbackURL,
		client:           &http.Client{Timeout: 10 * time.Second},
		lastHealthCheck:  make(map[string]time.Time),
		isDefaultHealthy: true,
		isFallbackHealthy: true,
	}
}

func (cb *CircuitBreaker) healthCheckRoutine() {
	ticker := time.NewTicker(6 * time.Second) // Check every 6 seconds to respect 5s limit
	defer ticker.Stop()
	
	for range ticker.C {
		cb.checkProcessorHealth("default", cb.defaultURL)
		time.Sleep(100 * time.Millisecond) // Small delay between checks
		cb.checkProcessorHealth("fallback", cb.fallbackURL)
	}
}

func (cb *CircuitBreaker) checkProcessorHealth(processor, url string) {
	cb.healthMutex.Lock()
	lastCheck, exists := cb.lastHealthCheck[processor]
	now := time.Now()
	
	// Respect 5-second rate limit per processor
	if exists && now.Sub(lastCheck) < 5*time.Second {
		cb.healthMutex.Unlock()
		return
	}
	
	cb.lastHealthCheck[processor] = now
	cb.healthMutex.Unlock()
	
	resp, err := cb.client.Get(url + "/payments/service-health")
	if err != nil {
		cb.updateHealthStatus(processor, false)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		cb.updateHealthStatus(processor, false)
		return
	}
	
	var health HealthCheck
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		cb.updateHealthStatus(processor, false)
		return
	}
	
	cb.updateHealthStatus(processor, !health.Failing)
}

func (cb *CircuitBreaker) updateHealthStatus(processor string, healthy bool) {
	cb.healthMutex.Lock()
	defer cb.healthMutex.Unlock()
	
	if processor == "default" {
		cb.isDefaultHealthy = healthy
	} else {
		cb.isFallbackHealthy = healthy
	}
}

func (cb *CircuitBreaker) selectProcessor() (string, string) {
	cb.healthMutex.RLock()
	defer cb.healthMutex.RUnlock()
	
	// Prefer default processor (5% fee) if healthy
	if cb.isDefaultHealthy {
		return "default", cb.defaultURL
	}
	
	// Fall back to fallback processor (15% fee) if available
	if cb.isFallbackHealthy {
		return "fallback", cb.fallbackURL
	}
	
	// If both are unhealthy, try default anyway
	return "default", cb.defaultURL
}

func (app *App) handlePayments(c *gin.Context) {
	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	
	// Validate UUID format
	if _, err := uuid.Parse(req.CorrelationID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid correlationId format"})
		return
	}
	
	// Process payment asynchronously
	go app.processPayment(req)
	
	c.JSON(http.StatusAccepted, gin.H{"message": "payment accepted"})
}

func (app *App) processPayment(req PaymentRequest) {
	processor, url := app.circuitBreaker.selectProcessor()
	
	requestedAt := time.Now().UTC()
	
	// Prepare request for payment processor
	processorReq := PaymentProcessorRequest{
		CorrelationID: req.CorrelationID,
		Amount:        req.Amount,
		RequestedAt:   requestedAt,
	}
	
	// Send to payment processor
	success := app.sendToProcessor(url, processorReq)
	
	// If default processor failed and we haven't tried fallback, try fallback
	if !success && processor == "default" {
		processor = "fallback"
		url = app.circuitBreaker.fallbackURL
		app.sendToProcessor(url, processorReq)
	}
	
	// Store in our database regardless of processor result for consistency
	app.storePayment(req.CorrelationID, req.Amount, requestedAt, processor)
}

func (app *App) sendToProcessor(url string, req PaymentProcessorRequest) bool {
	reqJSON, err := json.Marshal(req)
	if err != nil {
		log.Printf("Error marshaling request: %v", err)
		return false
	}
	
	resp, err := app.circuitBreaker.client.Post(url+"/payments", "application/json", 
		bytes.NewBuffer(reqJSON))
	if err != nil {
		log.Printf("Error sending to processor %s: %v", url, err)
		return false
	}
	defer resp.Body.Close()
	
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (app *App) storePayment(correlationID string, amount float64, requestedAt time.Time, processor string) {
	query := `INSERT INTO payments (correlation_id, amount, requested_at, processor) 
			  VALUES ($1, $2, $3, $4) ON CONFLICT (correlation_id) DO NOTHING`
	
	_, err := app.db.Exec(query, correlationID, amount, requestedAt, processor)
	if err != nil {
		log.Printf("Error storing payment: %v", err)
	}
}

func (app *App) handlePaymentsSummary(c *gin.Context) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	
	var fromTime, toTime *time.Time
	
	if fromStr != "" {
		if parsed, err := time.Parse(time.RFC3339, fromStr); err == nil {
			fromTime = &parsed
		}
	}
	
	if toStr != "" {
		if parsed, err := time.Parse(time.RFC3339, toStr); err == nil {
			toTime = &parsed
		}
	}
	
	summary, err := app.getPaymentsSummary(fromTime, toTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get payments summary"})
		return
	}
	
	c.JSON(http.StatusOK, summary)
}

func (app *App) getPaymentsSummary(from, to *time.Time) (*PaymentSummary, error) {
	baseQuery := `SELECT processor, COUNT(*), COALESCE(SUM(amount), 0) FROM payments`
	args := []interface{}{}
	argCount := 0
	
	conditions := []string{}
	
	if from != nil {
		argCount++
		conditions = append(conditions, "requested_at >= $"+strconv.Itoa(argCount))
		args = append(args, *from)
	}
	
	if to != nil {
		argCount++
		conditions = append(conditions, "requested_at <= $"+strconv.Itoa(argCount))
		args = append(args, *to)
	}
	
	if len(conditions) > 0 {
		baseQuery += " WHERE " + strings.Join(conditions, " AND ")
	}
	
	baseQuery += " GROUP BY processor"
	
	rows, err := app.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	summary := &PaymentSummary{}
	
	for rows.Next() {
		var processor string
		var count int
		var total float64
		
		if err := rows.Scan(&processor, &count, &total); err != nil {
			return nil, err
		}
		
		if processor == "default" {
			summary.Default.TotalRequests = count
			summary.Default.TotalAmount = total
		} else if processor == "fallback" {
			summary.Fallback.TotalRequests = count
			summary.Fallback.TotalAmount = total
		}
	}
	
	return summary, nil
}