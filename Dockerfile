FROM golang:1.23.5-alpine AS builder
WORKDIR /app
COPY . .
# RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -v -o /ichigod ./cmd/ichigod

FROM python:3-alpine
RUN pip install telegramify-markdown
COPY --from=builder /ichigod /usr/local/bin/ichigod
RUN chmod +x /usr/local/bin/ichigod

ENV ICHIGOD_DATA_DIR=/etc/ichigod
VOLUME /etc/ichigod
CMD ["ichigod"]
