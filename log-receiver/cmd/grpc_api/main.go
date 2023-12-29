package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	pb "log-api/logs"
	"log-receiver/internal/data"
	"log-receiver/internal/driver"

	"google.golang.org/grpc"
)

type logServiceServer struct {
	pb.UnimplementedLogServiceServer
	dataChannel chan data.Log
	models      data.Models
}

func (s *logServiceServer) flushDataToClickHouse() {
	buffer := make([]data.Log, 0, 100) // Change the buffer type to hold pointers
	for msg := range s.dataChannel {
		buffer = append(buffer, msg)
		fmt.Println("receive from channel", msg)

		if len(buffer) >= 1 {
			new_buff := make([]data.Log, 0, len(buffer))
			new_buff = append(new_buff, buffer...)
			fmt.Println("buffer -->>>", new_buff)

			go s.flushBufferToClickHouse(new_buff)
			buffer = buffer[:0] // Clear the buffer
		}
		// Connect to ClickHouse if not already connected
		// if s.clickHouseConn == nil {
		//     conn, err := clickhouse.Open("tcp://clickhouse-server:9000?username=default&password=yourpassword")
		//     if err != nil {
		//         log.Fatalf("Failed to connect to ClickHouse: %v", err)
		//     }
		//     s.clickHouseConn = conn
		// }

		// // Insert data into ClickHouse
		// _, err := s.clickHouseConn.Exec("INSERT INTO yourtable (column1, column2) VALUES (?, ?)", data.Column1, data.Column2)
		// if err != nil {
		//     log.Printf("Failed to insert data into ClickHouse: %v", err)
		// }
	}
}

func (s *logServiceServer) flushBufferToClickHouse(buffer []data.Log) {
	// Connect to ClickHouse if not already connected
	s.models.Log.InsertTestData(context.Background(), buffer)
}

func (s *logServiceServer) LogMessage(ctx context.Context, req *pb.LogMessageRequest) (*pb.LogMessageResponse, error) {
	// Handle incoming log message, e.g., save it to a file or a database
	log.Printf("Received log message: %s", req.Message)
	s.dataChannel <- data.Log{
		Timestamp: req.Timestamp.AsTime(),
		Message:   req.Message,
	}
	return &pb.LogMessageResponse{Success: true}, nil
}

func (s *logServiceServer) SendSpans(ctx context.Context, req *pb.Spans) (*pb.LogMessageResponse, error) {
	// Handle incoming log message, e.g., save it to a file or a database
	//log.Println(req.Spans)
	//s.dataChannel <- data.Log{
	//	Timestamp: req.Timestamp.AsTime(),
	//	Message:   req.Message,
	//}
	s.models.Log.InsertTraceData(context.Background(), req.Spans)

	return &pb.LogMessageResponse{Success: true}, nil
}

func main() {
	clickhouse, err := driver.Connect()

	ctx := context.Background()
	driver.CreateTracer(ctx, clickhouse)
	driver.СreateLogs(ctx, clickhouse)

	if err != nil {
		log.Fatalf("Failed to connect to clickhouse")
	}

	lis, err := net.Listen("tcp", ":50052") // Change the port as needed
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	srv := logServiceServer{
		models: data.New(clickhouse),
	}

	srv.dataChannel = make(chan data.Log, 1000)
	//go srv.flushDataToClickHouse()
	go func() {
		for {
			time.Sleep(time.Second * 5)
			srv.flushDataToClickHouse()
		}
	}()

	pb.RegisterLogServiceServer(grpcServer, &srv)

	log.Println("gRPC server listening on :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
