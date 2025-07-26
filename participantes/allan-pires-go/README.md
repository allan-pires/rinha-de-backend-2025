# Rinha de Backend 2025 - Go Implementation

This implementation provides a high-performance payment processing intermediary system using Go.

## Architecture

The solution implements a resilient payment processing backend that intermediates requests between clients and two payment processors with intelligent failover and optimal fee selection.

### Components
1. **Load Balancer (nginx)**: Distributes requests across multiple API instances
2. **API Instances (2x)**: Go applications using Gin framework for high-performance HTTP handling  
3. **Database (PostgreSQL)**: Stores payment records with proper indexing for fast queries
4. **Circuit Breaker**: Intelligent switching between payment processors based on health

### Implementation Details

**Payment Processing Logic:**
- Always prefer the default processor (5% fee) over fallback (15% fee)
- Health monitoring respects 5-second rate limits per processor
- Circuit breaker automatically switches to fallback when default processor fails
- Async processing handles payments without blocking API responses
- Database records ensure audit compliance between backend and payment processors

**Performance Optimizations:**
- Connection pooling for database and HTTP clients
- Minimal memory allocations with efficient JSON handling
- Optimized nginx configuration for high throughput
- Resource-constrained container limits (1.5 CPU, 280MB total)

**Resilience Features:**
- Health checking of payment processors with exponential backoff
- Automatic failover to fallback processor
- Database connection retry logic
- Graceful degradation under load

## API Endpoints

### POST /payments
Accepts payment requests and forwards to the optimal payment processor.

**Request:**
```json
{
    "correlationId": "4a7901b8-7d26-4d9d-aa19-4dc1c7cf60b3",
    "amount": 19.90
}
```

**Response:** HTTP 202 Accepted

### GET /payments-summary
Returns aggregated payment statistics by processor and time range.

**Query Parameters:**
- `from` (optional): ISO timestamp filter start
- `to` (optional): ISO timestamp filter end

**Response:**
```json
{
    "default": {
        "totalRequests": 43236,
        "totalAmount": 415542345.98
    },
    "fallback": {
        "totalRequests": 423545,
        "totalAmount": 329347.34
    }
}
```

## Resource Usage
- **Database:** 0.3 CPU, 100MB
- **API Instance 1:** 0.5 CPU, 80MB  
- **API Instance 2:** 0.5 CPU, 80MB
- **nginx:** 0.2 CPU, 20MB
- **Total:** 1.5 CPU, 280MB ✓

## Quick Start

1. Start payment processors:
```bash
cd payment-processor && docker compose up -d
```

2. Start the backend:
```bash
cd participantes/allan-pires-go && docker compose up
```

3. Test endpoints:
```bash
curl -X POST http://localhost:9999/payments \
  -H "Content-Type: application/json" \
  -d '{"correlationId":"550e8400-e29b-41d4-a716-446655440000","amount":100.00}'

curl http://localhost:9999/payments-summary
```