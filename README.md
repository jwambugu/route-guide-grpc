# Description

This is an implementation of a gRPC client and server that provides route guidance
from [gRPC Basics: Go](https://grpc.io/docs/tutorials/basic/go.html) tutorial.

It demonstrates how to use grpc go libraries to perform unary, client streaming, server streaming and full duplex RPCs.

The [original repository](https://github.com/grpc/grpc-go/tree/master/examples/route_guide) has more details.

## Run Locally

To run the server locally, run the following command:

```sh
go run server/server.go
```

To run the client locally, run the following command:

```sh
go run client/client.go
```