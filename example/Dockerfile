FROM golang:1.22 

COPY main.go go.mod go.sum .
RUN CGO_ENABLED=0 go build -o /bin/main ./main.go

FROM scratch
COPY --from=0 /bin/main /bin/main
ENTRYPOINT ["/bin/main"]
