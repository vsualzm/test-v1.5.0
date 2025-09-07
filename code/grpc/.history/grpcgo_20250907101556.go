
// buat dulu protonya
// 1) Protobuf (account.proto)
// syntax = "proto3";

// package account;
// option go_package = "example.com/microservices/accountpb";

// service AccountService {
//   rpc GetAccount(GetAccountRequest) returns (GetAccountResponse);
// }

// message GetAccountRequest {
//   string id = 1;
// }

// message GetAccountResponse {
//   string id = 1;
//   string name = 2;
// }


// 2) Server (Account Service)
package main

import (
	"context"
	"fmt"
	"log"
	"net"

	pb "example.com/microservices/accountpb"
	"google.golang.org/grpc"
)

type server struct {
	pb.UnimplementedAccountServiceServer
}

func (s *server) GetAccount(ctx context.Context, req *pb.GetAccountRequest) (*pb.GetAccountResponse, error) {
	return &pb.GetAccountResponse{
		Id:   req.Id,
		Name: fmt.Sprintf("User-%s", req.Id),
	}, nil
}

func main() {
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal("gagal listen:", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAccountServiceServer(grpcServer, &server{})

	log.Println("Account Service jalan di :50051")
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatal("gagal serve:", err)
	}
}



// client
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "example.com/microservices/accountpb"
	"google.golang.org/grpc"
)

func main() {
	// koneksi ke Account Service
	conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatal("gagal koneksi:", err)
	}
	defer conn.Close()

	client := pb.NewAccountServiceClient(conn)

	// panggil GetAccount
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	resp, err := client.GetAccount(ctx, &pb.GetAccountRequest{Id: "123"})
	if err != nil {
		log.Fatal("error panggil GetAccount:", err)
	}

	fmt.Println("Response dari Account Service:", resp.Id, resp.Name)
}

// OUTPUT
// Response dari Account Service: 123 User-123
