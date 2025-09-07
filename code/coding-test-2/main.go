// main.go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/streadway/amqp"
)

// --- Default Konfigurasi (bisa di-override via flag/env) ---
const (
	defaultRabbitURL      = "amqp://guest:guest@localhost:5672/"
	defaultRedisAddr      = "localhost:6379"
	defaultQueueName      = "report_requests"
	defaultNumRequests    = 10
	defaultNumWorkers     = 3
	defaultWorkerTimeoutS = 5
	defaultPublishMs      = 500
)

const (
	KEY_PREFIX_REPORT_STATUS = "report:status:" // string: "PENDING" | "IN_PROGRESS" | "COMPLETED" | "FAILED"
	KEY_PREFIX_REPORT_DATA   = "report:data:"   // json ReportResult
)

type ReportRequest struct {
	ID         string            `json:"id"`
	ReportType string            `json:"report_type"` // e.g., "sales", "inventory"
	Parameters map[string]string `json:"parameters"`
	CreatedAt  time.Time         `json:"created_at"`
}

type ReportStatus string

const (
	StatusPending    ReportStatus = "PENDING"
	StatusInProgress ReportStatus = "IN_PROGRESS"
	StatusCompleted  ReportStatus = "COMPLETED"
	StatusFailed     ReportStatus = "FAILED"
)

type ReportResult struct {
	RequestID   string       `json:"request_id"`
	Status      ReportStatus `json:"status"`
	GeneratedAt time.Time    `json:"generated_at"`
	ReportData  string       `json:"report_data,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// --- Flags/ENV ---
type Config struct {
	RabbitURL     string
	RedisAddr     string
	QueueName     string
	NumRequests   int
	NumWorkers    int
	WorkerTimeout time.Duration
	PublishEvery  time.Duration
}

func loadConfig() *Config {
	cfg := &Config{}
	rabbitURL := getenv("RABBITMQ_URL", defaultRabbitURL)
	redisAddr := getenv("REDIS_URL", defaultRedisAddr)
	queueName := getenv("QUEUE_NAME", defaultQueueName)

	// Flags
	flag.StringVar(&cfg.RabbitURL, "rabbit", rabbitURL, "RabbitMQ URL")
	flag.StringVar(&cfg.RedisAddr, "redis", redisAddr, "Redis address")
	flag.StringVar(&cfg.QueueName, "queue", queueName, "RabbitMQ queue name")
	flag.IntVar(&cfg.NumRequests, "n", defaultNumRequests, "number of report requests to produce")
	flag.IntVar(&cfg.NumWorkers, "workers", defaultNumWorkers, "number of concurrent workers")
	wt := flag.Int("worker_timeout_s", defaultWorkerTimeoutS, "per-task worker timeout (seconds)")
	pubms := flag.Int("publish_ms", defaultPublishMs, "interval between publishing requests (ms)")
	flag.Parse()

	cfg.WorkerTimeout = time.Duration(*wt) * time.Second
	cfg.PublishEvery = time.Duration(*pubms) * time.Millisecond
	return cfg
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// --- Koneksi RabbitMQ---
func connectRabbitMQ(ctx context.Context, url, queue string) (*amqp.Connection, *amqp.Channel, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, nil, fmt.Errorf("connect RabbitMQ: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("open channel: %w", err)
	}
	_, err = ch.QueueDeclare(
		queue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, nil, fmt.Errorf("declare queue: %w", err)
	}
	log.Printf("[RabbitMQ] Connected & queue declared: %s", queue)
	return conn, ch, nil
}

func connectRedis(ctx context.Context, addr string) *redis.Client {
	rdb := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   0,
	})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("connect Redis: %v", err)
	}
	log.Printf("[Redis] Connected: %s", addr)
	return rdb
}

func simulateReportGeneration(ctx context.Context, request ReportRequest) (string, error) {
	// Simulasi kerja 1-5 detik
	delay := time.Duration(1+rand.Intn(5)) * time.Second
	select {
	case <-time.After(delay):
		// continue
	case <-ctx.Done():
		return "", ctx.Err()
	}
	// 20% gagal
	if rand.Intn(100) < 20 {
		return "", fmt.Errorf("simulated report generation error for ID %s", request.ID)
	}
	reportData := fmt.Sprintf("Report %s - Type: %s, Generated At: %s, Data: %d",
		request.ID, request.ReportType, time.Now().Format(time.RFC3339), rand.Intn(1000))
	return reportData, nil
}

// 1) PRODUCER
func produceReportRequests(ctx context.Context, wg *sync.WaitGroup, ch *amqp.Channel, queue string, total int, interval time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for i := 0; i < total; i++ {
			select {
			case <-ctx.Done():
				log.Println("[Producer] context canceled, stop producing")
				return
			case <-ticker.C:
				req := ReportRequest{
					ID:         fmt.Sprintf("REQ-%03d", i+1),
					ReportType: []string{"sales", "inventory", "finance"}[rand.Intn(3)],
					Parameters: map[string]string{
						"date_from": time.Now().AddDate(0, 0, -7).Format("2006-01-02"),
						"date_to":   time.Now().Format("2006-01-02"),
					},
					CreatedAt: time.Now(),
				}
				body, _ := json.Marshal(req)
				err := ch.Publish(
					"",    // default exchange
					queue, // routing key = queue
					false, // mandatory
					false, // immediate
					amqp.Publishing{
						ContentType:  "application/json",
						DeliveryMode: amqp.Persistent, // durable message
						Body:         body,
					},
				)
				if err != nil {
					log.Printf("[Producer] publish error for %s: %v", req.ID, err)
					continue
				}
				log.Printf("[Producer] published request: %s", req.ID)
			}
		}
		log.Println("[Producer] finished producing all requests")
	}()
}

// 2) UPDATE STATUS KE REDIS
func updateReportStatus(ctx context.Context, rdb *redis.Client, requestID string, status ReportStatus, reportData string, errMsg string) error {
	// Simpan status saat ini
	if err := rdb.Set(ctx, KEY_PREFIX_REPORT_STATUS+requestID, string(status), 0).Err(); err != nil {
		return fmt.Errorf("redis SET status: %w", err)
	}

	// Jika completed/failed, simpan hasil lengkap
	if status == StatusCompleted || status == StatusFailed {
		result := ReportResult{
			RequestID:   requestID,
			Status:      status,
			GeneratedAt: time.Now(),
			ReportData:  reportData,
			Error:       errMsg,
		}
		js, _ := json.Marshal(result)
		if err := rdb.Set(ctx, KEY_PREFIX_REPORT_DATA+requestID, js, 0).Err(); err != nil {
			return fmt.Errorf("redis SET result: %w", err)
		}
	}
	return nil
}

// 3) WORKER: proses amqp.Delivery -> simulate -> kirim ReportResult
func reportWorker(ctx context.Context, workerID int, tasks <-chan amqp.Delivery, results chan<- ReportResult, rdb *redis.Client, workerTimeout time.Duration) {
	log.Printf("[Worker-%d] started", workerID)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker-%d] context canceled, exit", workerID)
			return
		case d, ok := <-tasks:
			if !ok {
				log.Printf("[Worker-%d] tasks channel closed, exit", workerID)
				return
			}
			var req ReportRequest
			if err := json.Unmarshal(d.Body, &req); err != nil {
				_ = updateReportStatus(ctx, rdb, "UNKNOWN", StatusFailed, "", "invalid JSON body")
				// Kirim result dengan RequestID unknown agar tetap di-handle (akan di-Ack oleh orchestrator berdasarkan map; kalau tak ada mapping, diabaikan)
				results <- ReportResult{RequestID: req.ID, Status: StatusFailed, Error: "invalid JSON"}
				continue
			}

			// Mark IN_PROGRESS
			_ = updateReportStatus(ctx, rdb, req.ID, StatusInProgress, "", "")

			// Per-task ctx dengan timeout
			tctx, cancel := context.WithTimeout(ctx, workerTimeout)
			data, err := simulateReportGeneration(tctx, req)
			cancel()

			if err != nil {
				_ = updateReportStatus(ctx, rdb, req.ID, StatusFailed, "", err.Error())
				results <- ReportResult{RequestID: req.ID, Status: StatusFailed, Error: err.Error(), GeneratedAt: time.Now()}
				continue
			}

			_ = updateReportStatus(ctx, rdb, req.ID, StatusCompleted, data, "")
			results <- ReportResult{RequestID: req.ID, Status: StatusCompleted, ReportData: data, GeneratedAt: time.Now()}
		}
	}
}

// helper: thread-safe map requestID -> delivery
type deliveryMap struct {
	mu   sync.RWMutex
	data map[string]amqp.Delivery
}

func newDeliveryMap() *deliveryMap {
	return &deliveryMap{data: make(map[string]amqp.Delivery)}
}
func (m *deliveryMap) set(id string, d amqp.Delivery) {
	m.mu.Lock()
	m.data[id] = d
	m.mu.Unlock()
}
func (m *deliveryMap) pop(id string) (amqp.Delivery, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.data[id]
	if ok {
		delete(m.data, id)
	}
	return d, ok
}

// 4) ACK HANDLER
func resultAckHandler(ctx context.Context, results <-chan ReportResult, ch *amqp.Channel, rdb *redis.Client, dmap *deliveryMap) {
	log.Println("[AckHandler] started")
	for {
		select {
		case <-ctx.Done():
			log.Println("[AckHandler] context canceled, exit")
			return
		case res, ok := <-results:
			if !ok {
				log.Println("[AckHandler] results channel closed, exit")
				return
			}

			// Cari delivery berdasarkan RequestID
			d, found := dmap.pop(res.RequestID)
			if !found {
				// Bisa terjadi kalau JSON invalid / mapping tak tercatat
				log.Printf("[AckHandler] no delivery mapping for requestID=%s (already handled or invalid)", res.RequestID)
				continue
			}

			switch res.Status {
			case StatusCompleted, StatusFailed:
				// Sesuai soal: treat semua kegagalan sebagai fatal -> Ack (tidak requeue)
				if err := d.Ack(false); err != nil {
					log.Printf("[AckHandler] Ack error for %s: %v", res.RequestID, err)
				} else {
					log.Printf("[AckHandler] Acked message for %s (status=%s)", res.RequestID, res.Status)
				}
			default:
				// Normalnya tidak masuk ke sini (ack saat final only)
				if err := d.Ack(false); err != nil {
					log.Printf("[AckHandler] Ack (default) error for %s: %v", res.RequestID, err)
				} else {
					log.Printf("[AckHandler] Acked (default) %s", res.RequestID)
				}
			}
		}
	}
}

// 5) ORCHESTRATOR CONSUMER
func startReportProcessor(ctx context.Context, wg *sync.WaitGroup, rdb *redis.Client, rabbitURL, queue string, numWorkers int, workerTimeout time.Duration) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, ch, err := connectRabbitMQ(ctx, rabbitURL, queue)
		if err != nil {
			log.Printf("[Consumer] rabbit connect error: %v", err)
			return
		}
		defer func() {
			_ = ch.Close()
			_ = conn.Close()
			log.Println("[Consumer] rabbit closed")
		}()

		// Consume dengan autoAck=false (manual ack)
		deliveries, err := ch.Consume(
			queue,
			"",    // consumer tag
			false, // auto-ack
			false, // exclusive
			false, // no-local (deprecated)
			false, // no-wait
			nil,   // args
		)
		if err != nil {
			log.Printf("[Consumer] ch.Consume error: %v", err)
			return
		}

		// Channels
		tasks := make(chan amqp.Delivery, numWorkers*2)
		results := make(chan ReportResult, numWorkers*2)

		// Delivery map
		dmap := newDeliveryMap()

		// Start workers
		workerCtx, workerCancel := context.WithCancel(ctx)
		var wwg sync.WaitGroup
		for i := 1; i <= numWorkers; i++ {
			wwg.Add(1)
			go func(id int) {
				defer wwg.Done()
				reportWorker(workerCtx, id, tasks, results, rdb, workerTimeout)
			}(i)
		}

		// Start ack handler
		var ackWG sync.WaitGroup
		ackWG.Add(1)
		go func() {
			defer ackWG.Done()
			resultAckHandler(workerCtx, results, ch, rdb, dmap)
		}()

		log.Printf("[Consumer] processor started with %d workers", numWorkers)

		// Loop utama penerima delivery
	loop:
		for {
			select {
			case <-ctx.Done():
				log.Println("[Consumer] context canceled, draining...")
				break loop
			case d, ok := <-deliveries:
				if !ok {
					log.Println("[Consumer] deliveries channel closed by server")
					break loop
				}

				// Parse untuk ambil RequestID (butuh untuk set PENDING & mapping)
				var req ReportRequest
				if err := json.Unmarshal(d.Body, &req); err != nil {
					// invalid payload -> mark failed & Ack
					log.Printf("[Consumer] invalid JSON, acking message: %v", err)
					_ = updateReportStatus(ctx, rdb, "UNKNOWN", StatusFailed, "", "invalid JSON at consumer")
					_ = d.Ack(false)
					continue
				}

				// Tandai PENDING
				_ = updateReportStatus(ctx, rdb, req.ID, StatusPending, "", "")

				// Set mapping (supaya ack handler tahu delivery mana)
				dmap.set(req.ID, d)

				// Teruskan ke worker
				select {
				case tasks <- d:
				case <-ctx.Done():
					log.Println("[Consumer] context canceled while enqueueing task")
					break loop
				}
			}
		}

		// Shutdown sequence
		// 1) Stop workers dan ack handler
		workerCancel()
		close(tasks)
		wwg.Wait()
		close(results)
		ackWG.Wait()

		log.Println("[Consumer] processor stopped gracefully")
	}()
}

// --- MAIN ---
func main() {
	rand.Seed(time.Now().UnixNano())
	cfg := loadConfig()

	// Root context + signal cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handler
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Koneksi shared
	rdb := connectRedis(ctx, cfg.RedisAddr)
	rabbitConn, rabbitCh, err := connectRabbitMQ(ctx, cfg.RabbitURL, cfg.QueueName)
	if err != nil {
		log.Fatalf("RabbitMQ connection failed: %v", err)
	}
	defer func() {
		_ = rabbitCh.Close()
		_ = rabbitConn.Close()
	}()

	var wg sync.WaitGroup

	// Start producer
	produceReportRequests(ctx, &wg, rabbitCh, cfg.QueueName, cfg.NumRequests, cfg.PublishEvery)

	// Start consumer/processor
	startReportProcessor(ctx, &wg, rdb, cfg.RabbitURL, cfg.QueueName, cfg.NumWorkers, cfg.WorkerTimeout)

	// Wait signals
	select {
	case s := <-sigCh:
		log.Printf("[Main] received signal: %s", s)
		cancel()
	case <-ctx.Done():
	}

	// Tunggu semua goroutine selesai
	wg.Wait()
	log.Println("[Main] shutdown complete")
}

// --- (opsional) helper error check ---
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	return false
}
