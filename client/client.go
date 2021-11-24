package main

import (
	"context"
	"flag"
	routeguide "github.com/jwambugu/routeguide/protos"
	"google.golang.org/grpc"
	"io"
	"log"
	"math/rand"
	"time"
)

var (
	serverAddr = flag.String("addr", "localhost:50051", "The server address in the format of host:port")
)

// printFeature gets the feature for the given point.
func printFeature(client routeguide.RouteGuideClient, point *routeguide.Point) {
	log.Printf("getting feature for point (%d, %d)\n", point.Latitude, point.Longitude)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	feature, err := client.GetFeature(ctx, point)
	if err != nil {
		log.Fatalf("%v.GetFeatures(_) = _, %v: ", client, err)
	}

	log.Println(feature)
}

// printFeatures lists all the features within the given bounding Rectangle.
func printFeatures(client routeguide.RouteGuideClient, rectangle *routeguide.Rectangle) {
	log.Printf("looking for features within %v\n", rectangle)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.ListFeatures(ctx, rectangle)
	if err != nil {
		log.Fatalf("%v.ListFeatures(_) = _, %v", client, err)
	}

	for {
		feature, err := stream.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("%v.ListFeatures(_) = _, %v", client, err)
		}

		log.Printf("Feature: name: %q, point:(%v, %v)",
			feature.GetName(),
			feature.GetLocation().GetLatitude(),
			feature.GetLocation().GetLongitude(),
		)
	}
}

func randomPoint(r *rand.Rand) *routeguide.Point {
	lat := (r.Int31n(180) - 90) * 1e7
	long := (r.Int31n(360) - 180) * 1e7

	return &routeguide.Point{Latitude: lat, Longitude: long}
}

// runRecordRoute sends a sequence of points to server and expects to get a RouteSummary from server.
func runRecordRoute(client routeguide.RouteGuideClient) {
	// Create a random number of random points
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	pointCount := int(r.Int31n(100)) + 2 // Traverse at least two points
	var points []*routeguide.Point

	for i := 0; i < pointCount; i++ {
		points = append(points, randomPoint(r))
	}

	log.Printf("traversing %d points.", len(points))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.RecordRoute(ctx)
	if err != nil {
		log.Fatalf("%v.RecordRoute(_) = _, %v", client, err)
	}

	for _, point := range points {
		if err := stream.Send(point); err != nil {
			log.Fatalf("%v.Send(%v) = %v", stream, point, err)
		}
	}

	reply, err := stream.CloseAndRecv()
	if err != nil {
		log.Fatalf("%v.CloseAndRecv() got error %v, want %v", stream, err, nil)
	}

	log.Printf("route summary: %+v", reply)
}

// runRouteChat receives a sequence of route notes, while sending notes for various locations.
func runRouteChat(client routeguide.RouteGuideClient) {
	notes := []*routeguide.RouteNote{
		{Location: &routeguide.Point{Latitude: 0, Longitude: 1}, Message: "First message"},
		{Location: &routeguide.Point{Latitude: 0, Longitude: 2}, Message: "Second message"},
		{Location: &routeguide.Point{Latitude: 0, Longitude: 3}, Message: "Third message"},
		{Location: &routeguide.Point{Latitude: 0, Longitude: 1}, Message: "Fourth message"},
		{Location: &routeguide.Point{Latitude: 0, Longitude: 2}, Message: "Fifth message"},
		{Location: &routeguide.Point{Latitude: 0, Longitude: 3}, Message: "Sixth message"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := client.RouteChat(ctx)
	if err != nil {
		log.Fatalf("%v.RouteChat(_) = _, %v", client, err)
	}

	waitChan := make(chan struct{})
	go func() {
		for {
			in, err := stream.Recv()
			if err == io.EOF {
				close(waitChan)
				return
			}

			if err != nil {
				log.Fatalf("Failed to receive a note : %v", err)
			}

			log.Printf("Got message %s at point(%d, %d)", in.Message, in.Location.Latitude, in.Location.Longitude)
		}
	}()

	for _, note := range notes {
		if err := stream.Send(note); err != nil {
			log.Fatalf("Failed to send a note: %v", err)
		}
	}
	stream.CloseSend()
	<-waitChan
}

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*serverAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}

	defer func(conn *grpc.ClientConn) {
		_ = conn.Close()
	}(conn)

	client := routeguide.NewRouteGuideClient(conn)

	// Looking for a valid feature
	printFeature(client, &routeguide.Point{Latitude: 409146138, Longitude: -746188906})

	// Looking for a missing feature
	printFeature(client, &routeguide.Point{Latitude: 0, Longitude: 0})

	// Looking for features between 40, -75 and 42, -73.
	printFeatures(client, &routeguide.Rectangle{
		Lo: &routeguide.Point{Latitude: 400000000, Longitude: -750000000},
		Hi: &routeguide.Point{Latitude: 420000000, Longitude: -730000000},
	})

	runRecordRoute(client)

	runRouteChat(client)
}
