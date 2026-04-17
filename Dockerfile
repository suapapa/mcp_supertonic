# syntax=docker/dockerfile:1
FROM golang:bookworm AS builder

# Install git-lfs for downloading models
RUN apt-get update && apt-get install -y git-lfs && git lfs install

WORKDIR /app

# Copy the download script first to cache the model download layer
# This avoids re-downloading GBs of models when only source code changes
COPY scripts/download_supertonic.sh scripts/
RUN scripts/download_supertonic.sh

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code and build the binary
COPY . .
RUN go build -o /app/mcp_supertonic .

# Final stage - using debian slim for a smaller final image
FROM debian:bookworm-slim

# Install runtime dependencies:
# - ca-certificates for network requests
# - wget & tar for downloading onnxruntime
# - libgomp1 which is often required by ONNX Runtime on Linux
RUN apt-get update && apt-get install -y \
    ca-certificates \
    wget \
    tar \
    libgomp1 \
    && rm -rf /var/lib/apt/lists/*

# Download and install ONNX Runtime shared library
ENV ONNX_VERSION=1.17.1
RUN wget -q https://github.com/microsoft/onnxruntime/releases/download/v${ONNX_VERSION}/onnxruntime-linux-x64-${ONNX_VERSION}.tgz && \
    tar -xzf onnxruntime-linux-x64-${ONNX_VERSION}.tgz && \
    cp onnxruntime-linux-x64-${ONNX_VERSION}/lib/libonnxruntime.so* /usr/lib/ && \
    rm -rf onnxruntime-linux-x64-${ONNX_VERSION}*

# Set ONNX Runtime path for the supertonic engine
ENV ONNXRUNTIME_LIB_PATH=/usr/lib/libonnxruntime.so.1.17.1

# Copy the models from the builder stage
COPY --from=builder /root/.local/share/supertonic2 /root/.local/share/supertonic2

# Copy the compiled binary
COPY --from=builder /app/mcp_supertonic /usr/local/bin/

# Create a workspace directory for output files
WORKDIR /workspace

# Expose the default SSE port
EXPOSE 8080

# Entrypoint running the server in SSE mode
ENTRYPOINT ["mcp_supertonic", "-port", "8080"]
