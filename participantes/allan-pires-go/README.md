# Rinha de Backend 2025 - Go Implementation

## Repository
https://github.com/allan-pires/rinha-backend-2025-go

## Technologies Used
- **Language:** Go
- **Web Framework:** Gin
- **Database:** PostgreSQL
- **Load Balancer:** nginx
- **Containerization:** Docker

## Architecture

This implementation uses a resilient architecture designed for high performance and availability:

### Components
1. **Load Balancer (nginx)**: Distributes requests across multiple API instances
2. **API Instances (2x)**: Go applications using Gin framework for high-performance HTTP handling
3. **Database (PostgreSQL)**: Stores payment records with proper indexing for fast queries
4. **Circuit Breaker**: Intelligent switching between payment processors based on health

### Strategy

The implementation focuses on:
- **Optimal Fee Management**: Always prefer the default processor (5% fee) over fallback (15% fee)
- **Health Monitoring**: Periodically check processor health (respecting 5-second rate limits)
- **Circuit Breaker Pattern**: Automatically switch to fallback when default processor fails
- **Async Processing**: Handle payments asynchronously for better throughput
- **Data Consistency**: Ensure audit compliance between backend and payment processors

### Performance Optimizations
- Connection pooling for database and HTTP clients
- Minimal memory allocations
- Efficient JSON marshaling/unmarshaling
- Optimized nginx configuration
- Resource-constrained container limits

### Resource Usage
- Total CPU: 1.5 cores
- Total Memory: 200MB
- Network: Bridge mode with external payment-processor network

## Endpoints

### POST /payments
Accepts payment requests and forwards to the optimal payment processor.

### GET /payments-summary
Returns aggregated payment statistics by processor and time range.

## Resilience Features
- Health checking of payment processors
- Automatic failover to fallback processor
- Circuit breaker with exponential backoff
- Database connection retry logic
- Graceful degradation under load