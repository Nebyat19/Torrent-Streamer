FROM ubuntu:22.04 AS builder
WORKDIR /app

# Install Go 1.23.10 manually
RUN apt-get update && apt-get install -y wget && \
    wget https://go.dev/dl/go1.23.10.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && tar -C /usr/local -xzf go1.23.10.linux-amd64.tar.gz
ENV PATH="/usr/local/go/bin:${PATH}"

COPY . .
RUN go build -o /app/main .