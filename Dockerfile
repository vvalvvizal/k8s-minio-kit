FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/storage ./...

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/storage /storage
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/storage"]
