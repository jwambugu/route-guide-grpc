package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	routeguide "github.com/jwambugu/routeguide/protos"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net"
	"sync"
	"time"
)

var (
	jsonDBFile = flag.String("json_db_file", "testdata/route_guide_db.json", "A json file containing a list of features")
	port       = flag.Int("port", 50051, "The server port")
)

type routeGuideServer struct {
	routeguide.UnimplementedRouteGuideServer
	savedFeatures []*routeguide.Feature

	mu         sync.Mutex
	routeNotes map[string][]*routeguide.RouteNote
}

func inRange(point *routeguide.Point, rectangle *routeguide.Rectangle) bool {
	left := math.Min(float64(rectangle.Lo.Longitude), float64(rectangle.Hi.Longitude))
	right := math.Max(float64(rectangle.Lo.Longitude), float64(rectangle.Hi.Longitude))
	top := math.Max(float64(rectangle.Lo.Latitude), float64(rectangle.Hi.Latitude))
	bottom := math.Min(float64(rectangle.Lo.Latitude), float64(rectangle.Hi.Latitude))

	if float64(point.Longitude) >= left && float64(point.Longitude) <= right &&
		float64(point.Latitude) >= bottom && float64(point.Latitude) <= top {
		return true
	}

	return false
}

// toRadians converts an angle in degrees to radians.
func toRadians(num float64) float64 {
	return num * math.Pi / float64(180)
}

// calcDistance calculates the distance between two points using the "haversine" formula.
func calcDistance(p1 *routeguide.Point, p2 *routeguide.Point) int32 {
	const cordFactor float64 = 1e7
	const radiusInMeters = float64(6371000)

	lat1 := toRadians(float64(p1.Latitude) / cordFactor)
	lat2 := toRadians(float64(p2.Latitude) / cordFactor)
	lng1 := toRadians(float64(p1.Longitude) / cordFactor)
	lng2 := toRadians(float64(p2.Longitude) / cordFactor)

	dLat := lat2 - lat1
	dLng := lng2 - lng1

	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return int32(radiusInMeters * c)
}

func serialize(point *routeguide.Point) string {
	return fmt.Sprintf("%d %d", point.Latitude, point.Longitude)
}

// GetFeature returns the feature at the given point.
func (s *routeGuideServer) GetFeature(ctx context.Context, point *routeguide.Point) (*routeguide.Feature, error) {
	for _, feature := range s.savedFeatures {
		if proto.Equal(feature.Location, point) {
			return feature, nil
		}
	}
	// No feature was found, return an unnamed feature
	return &routeguide.Feature{Location: point}, nil
}

// ListFeatures lists all features contained within the given bounding Rectangle.
func (s *routeGuideServer) ListFeatures(rectangle *routeguide.Rectangle, stream routeguide.RouteGuide_ListFeaturesServer) error {
	for _, feature := range s.savedFeatures {
		if inRange(feature.Location, rectangle) {
			if err := stream.Send(feature); err != nil {
				return err
			}
		}
	}

	return nil
}

// RecordRoute records a route composited of a sequence of points.
//
// It gets a stream of points, and responds with statistics about the "trip":
// number of points,  number of known features visited, total distance traveled, and total time spent.
func (s *routeGuideServer) RecordRoute(stream routeguide.RouteGuide_RecordRouteServer) error {
	var pointCount, featureCount, distance int32
	var lastPoint *routeguide.Point
	startTime := time.Now()

	for {
		point, err := stream.Recv()
		if err == io.EOF {
			endTime := time.Now()

			return stream.SendAndClose(&routeguide.RouteSummary{
				PointCount:   pointCount,
				FeatureCount: featureCount,
				Distance:     distance,
				ElapsedTime:  int32(endTime.Sub(startTime).Seconds()),
			})
		}

		if err != nil {
			return err
		}

		pointCount++
		for _, feature := range s.savedFeatures {
			if proto.Equal(feature.Location, point) {
				featureCount++
			}
		}

		if lastPoint != nil {
			distance += calcDistance(lastPoint, point)
		}

		lastPoint = point
	}
}

// RouteChat receives a stream of message/location pairs, and responds with a stream of all
// previous messages at each of those locations.
func (s *routeGuideServer) RouteChat(stream routeguide.RouteGuide_RouteChatServer) error {
	for {
		input, err := stream.Recv()
		if err == io.EOF {
			return nil
		}

		if err != nil {
			return err
		}

		key := serialize(input.Location)

		s.mu.Lock()
		s.routeNotes[key] = append(s.routeNotes[key], input)

		routeNotes := make([]*routeguide.RouteNote, len(s.routeNotes[key]))
		copy(routeNotes, s.routeNotes[key])

		s.mu.Unlock()

		for _, note := range s.routeNotes[key] {
			if err := stream.Send(note); err != nil {
				return err
			}
		}
	}
}

// loadFeatures loads features from a JSON file.
func (s *routeGuideServer) loadFeatures(filepath string) {
	var data []byte

	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		log.Fatalf("failed to load default features: %v", err)
	}

	if err := json.Unmarshal(data, &s.savedFeatures); err != nil {
		log.Fatalf("failed to load default features: %v", err)
	}
}

func newRouteGuideServer() *routeGuideServer {
	server := &routeGuideServer{
		routeNotes: make(map[string][]*routeguide.RouteNote),
	}

	server.loadFeatures(*jsonDBFile)
	return server
}

func main() {

	flag.Parse()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	var opts []grpc.ServerOption

	grpcServer := grpc.NewServer(opts...)
	routeguide.RegisterRouteGuideServer(grpcServer, newRouteGuideServer())

	log.Println("Starting server on port:", *port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to start grpc server: %v", err)
	}
}
