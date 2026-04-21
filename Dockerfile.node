FROM golang:1.25.1

WORKDIR /app
COPY . .

RUN go mod download

EXPOSE 5000

CMD ["go", "run", "src/node/main/main.go", ":5000"]
