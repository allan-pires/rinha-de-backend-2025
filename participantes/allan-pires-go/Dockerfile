# Note: In the actual deployment, a proper multi-stage build would be used.
# Due to network restrictions in this environment, we're using a simple approach.
# In production, this would download dependencies and build in the container.

FROM busybox:latest

WORKDIR /app

# In production, copy the built binary from a proper Go build stage
# For demo purposes, this requires a pre-built binary named 'app'
COPY app /app/

# Make executable
RUN chmod +x /app/app

# Expose port
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/app/app"]