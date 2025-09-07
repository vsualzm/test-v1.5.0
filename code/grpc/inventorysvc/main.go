package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"inventorypb"

	"google.golang.org/grpc"
)

type server struct {
	inventorypb.UnimplementedInventoryServiceServer
	stock map[string]uint32
}

func (s *server) CheckStock(ctx context.Context, req *inventorypb.CheckStockRequest) (*inventorypb.CheckStockResponse, error) {
	qty := req.GetQty()
	remain, ok := s.stock[req.GetSku()]
	if !ok {
		return &inventorypb.CheckStockResponse{Available: false, Remaining: 0}, nil
	}
	available := remain >= qty
	var newRemain = remain
	if available {
		newRemain = remain - qty // simulasi hold stok
	}
	return &inventorypb.CheckStockResponse{
		Available: available,
		Remaining: newRemain,
	}, nil
}

func main() {
	// seed stok
	svc := &server{
		stock: map[string]uint32{
			"SKU-ABC": 10,
			"SKU-XYZ": 0,
		},
	}

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	grpcSrv := grpc.NewServer()
	inventorypb.RegisterInventoryServiceServer(grpcSrv, svc)

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Println("Inventory gRPC running on :50051")
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	<-stop
	log.Println("Shutting down Inventory...")
	grpcSrv.GracefulStop()
	fmt.Println("Stopped.")
}
