FROM golang:1.26.4-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:3.20
WORKDIR /app
COPY --from=build /app/server .
COPY --from=build /app/openapi.yaml .
COPY --from=build /app/schema.sql .
EXPOSE 6996
CMD ["./server"]