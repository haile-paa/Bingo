# 1) build the React frontend
FROM node:20-alpine AS web
WORKDIR /src/frontend
COPY frontend/package*.json ./
RUN npm install
COPY frontend/ ./
RUN npm run build

# 2) build the Go backend
FROM golang:1.23-alpine AS api
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o /out/bingo .

# 3) small runtime image (the backend serves ../frontend/dist)
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app/backend
COPY --from=api /out/bingo ./bingo
COPY --from=web /src/frontend/dist /app/frontend/dist
EXPOSE 10000
CMD ["./bingo"]
