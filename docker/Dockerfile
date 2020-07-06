# Build binary stage
FROM golang:1.14 as builder
WORKDIR /go/src/ec2-directory-to-s3-sync/
ENV CGO_ENABLED=0 GOOS=linux
COPY . .
RUN make build

# Build image stage
FROM alpine:3.12
ARG binaryName
ENV EC2_DIRECTORY_TO_S3_SYNC_PATH=/bin/${binaryName}
COPY --from=builder /go/src/ec2-directory-to-s3-sync/bin/${binaryName} ${EC2_DIRECTORY_TO_S3_SYNC_PATH}
COPY docker/entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]