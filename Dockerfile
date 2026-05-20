FROM golang:1.25-alpine AS build

RUN apk add --no-cache git
WORKDIR /app

COPY go.mod .
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/api.exe .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /bin/sidecar.exe ./sidecar

FROM alpine:3.18
WORKDIR /app
COPY --from=build /bin/api.exe /bin/api.exe
COPY --from=build /bin/sidecar.exe /bin/sidecar.exe

EXPOSE 9999 9998

CMD ["/bin/api.exe"]
