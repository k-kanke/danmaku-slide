# Multi-stage build for SlideFlow backend

FROM golang:1.22 AS build
WORKDIR /app

# Cache deps first
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Build
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /server .

# Minimal runtime image
FROM gcr.io/distroless/static-debian12
COPY --from=build /server /server

ENV PORT=8080
EXPOSE 8080

USER nonroot:nonroot
ENTRYPOINT ["/server"]

