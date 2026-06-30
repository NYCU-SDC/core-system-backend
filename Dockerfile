FROM golang:1.26.2

WORKDIR /app

COPY bin/backend /app/backend
COPY internal/database/migrations /app/migrations

EXPOSE 8080

CMD ["/app/backend"]
