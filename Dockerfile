FROM golang:1.23-alpine AS builder  
WORKDIR /app
COPY . . 
RUN go build -o /app/main .

