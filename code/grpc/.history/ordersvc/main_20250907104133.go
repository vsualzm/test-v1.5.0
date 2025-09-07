package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"inventorypb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CreateOrderReq struct {
	SKU string `json:"sku"`
	Qty uint32 `json:"qty"`
}

type CreateOrderResp struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	Remaining uint32 `json:"remaining,omitempty"`
}

func main() {
	// init gRPC client connection (with retry-like backoff via WithBlock + timeout)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		log.Fatalf("connect inventory: %v", err)
	}
	defer conn.Close()

	invCli := inventorypb.NewInventoryServiceClient(conn)

	mux := http.NewServeMux()
	mux.HandleFunc("/orders/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req CreateOrderReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		// panggil service Inventory dengan timeout per-request
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()

		res, err := invCli.CheckStock(ctx, &inventorypb.CheckStockRequest{
			Sku: req.SKU,
			Qty: req.Qty,
		})
		if err != nil {
			status := http.StatusBadGateway
			var msg = "inventory unavailable"
			// bisa tambahkan mapping error gRPC -> HTTP lebih detail
			http.Error(w, msg, status)
			return
		}

		if !res.GetAvailable() {
			writeJSON(w, http.StatusConflict, CreateOrderResp{
				Status:  "FAILED",
				Message: "Insufficient stock",
			})
			return
		}

		// Di sini normalnya kita lanjut simpan Order ke DB, publish event, dsb.
		writeJSON(w, http.StatusOK, CreateOrderResp{
			Status:    "OK",
			Message:   "Order created",
			Remaining: res.GetRemaining(),
		})
	})

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      logging(mux),
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// graceful shutdown
	go func() {
		log.Println("Order HTTP running on :8080")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down Order...")
	shCtx, shCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer shCancel()
	_ = srv.Shutdown(shCtx)
	log.Println("Stopped.")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}
