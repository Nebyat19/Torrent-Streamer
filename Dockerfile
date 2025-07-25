FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . . 
RUN go build -o /app/main .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/main /app/main
COPY --from=builder /app/static /app/static  
COPY --from=builder /app/templates /app/templates
CMD ["/app/main"]