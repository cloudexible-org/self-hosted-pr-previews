FROM golang:1.27-alpine AS build
ENV CGO_ENABLED=0
WORKDIR /src
COPY . .
RUN go vet ./... && go test ./...
RUN go build -trimpath -ldflags="-s -w" -o /out/dashboard .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/dashboard /dashboard
EXPOSE 8080
ENTRYPOINT ["/dashboard"]
