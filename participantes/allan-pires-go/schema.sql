-- Database schema for payment processing
CREATE TABLE IF NOT EXISTS payments (
    correlation_id UUID PRIMARY KEY,
    amount DECIMAL(15,2) NOT NULL,
    requested_at TIMESTAMP NOT NULL,
    processor VARCHAR(20) NOT NULL, -- 'default' or 'fallback'
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Index for efficient querying by timestamp range
CREATE INDEX IF NOT EXISTS idx_payments_requested_at ON payments(requested_at);
CREATE INDEX IF NOT EXISTS idx_payments_processor ON payments(processor);