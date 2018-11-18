FROM alpine:3.7
COPY crd-controller-sample /app/controller-main
ENTRYPOINT ["/app/controller-main"]
