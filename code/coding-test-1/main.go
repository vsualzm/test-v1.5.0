// main.go
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"
)

// TATA CARA MENGUJI 2 skenario
// time go run . -concurrency=1 -timeout_ms=2000 -global_timeout_ms=10000
// time go run . -concurrency=5 -timeout_ms=2000 -global_timeout_ms=10000

type ItemDetails struct {
	ID          string
	Name        string
	Description string
	Price       float64
}

func simulateFetchItemDetails(ctx context.Context, itemID string) (*ItemDetails, error) {
	delay := time.Duration(500+rand.Intn(1500)) * time.Millisecond

	select {
	case <-time.After(delay):
	case <-ctx.Done():
		log.Printf("[cancelled] item=%s, reason=%v", itemID, ctx.Err())
		return nil, ctx.Err()
	}

	if rand.Intn(100) < 15 {
		err := fmt.Errorf("simulated API error for item %s: service unavailable", itemID)
		log.Printf("[error] %v", err)
		return nil, err
	}

	d := &ItemDetails{
		ID:          itemID,
		Name:        fmt.Sprintf("Product %s", itemID),
		Description: fmt.Sprintf("Detailed description for product %s.", itemID),
		Price:       rand.Float64() * 100,
	}
	log.Printf("[ok] fetched %s (%.2f)", itemID, d.Price)
	return d, nil
}

func FetchAndAggregate(
	ctx context.Context,
	itemIDs []string,
	maxConcurrent int,
	perItemTimeout time.Duration,
) (map[string]ItemDetails, []error) {
	if len(itemIDs) == 0 {
		return map[string]ItemDetails{}, nil
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if perItemTimeout <= 0 {
		return nil, []error{errors.New("perItemTimeout must be > 0")}
	}

	results := make(map[string]ItemDetails, len(itemIDs))
	var (
		wg     sync.WaitGroup
		muRes  sync.Mutex
		errs   []error
		muErrs sync.Mutex
	)

	sem := make(chan struct{}, maxConcurrent)

	launch := func(id string) {
		sem <- struct{}{}
		wg.Add(1)
		go func(itemID string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			itemCtx, cancel := context.WithTimeout(ctx, perItemTimeout)
			defer cancel()

			d, err := simulateFetchItemDetails(itemCtx, itemID)
			if err != nil {
				muErrs.Lock()
				errs = append(errs, fmt.Errorf("item %s: %w", itemID, err))
				muErrs.Unlock()
				return
			}

			muRes.Lock()
			results[itemID] = *d
			muRes.Unlock()
		}(id)
	}

Loop:
	for _, id := range itemIDs {
		select {
		case <-ctx.Done():
			muErrs.Lock()
			errs = append(errs, fmt.Errorf("global context cancelled: %w", ctx.Err()))
			muErrs.Unlock()
			break Loop
		default:
			launch(id)
		}
	}

	wg.Wait()
	return results, errs
}

func main() {
	rand.Seed(time.Now().UnixNano())

	var (
		concurrency = flag.Int("concurrency", 3, "max concurrent requests")
		timeoutMS   = flag.Int("timeout_ms", 900, "per-item timeout in milliseconds")
		globalTOms  = flag.Int("global_timeout_ms", 5000, "global timeout in milliseconds")
		idsCSV      = flag.String("ids", "A1,A2,A3,A4,A5,A6,A7", "comma-separated item IDs")
		verbose     = flag.Bool("v", true, "verbose logging")
	)
	flag.Parse()

	if !*verbose {
		log.SetFlags(0)
		log.SetOutput(nil)
	}

	itemIDs := splitCSV(*idsCSV)

	globalCtx, cancel := context.WithTimeout(context.Background(), time.Duration(*globalTOms)*time.Millisecond)
	defer cancel()

	res, errs := FetchAndAggregate(globalCtx, itemIDs, *concurrency, time.Duration(*timeoutMS)*time.Millisecond)

	fmt.Println("=== Aggregation Result ===")
	for id, d := range res {
		fmt.Printf("- %s: %s | %.2f\n", id, d.Name, d.Price)
	}

	if len(errs) > 0 {
		fmt.Println("\n=== Errors ===")
		for _, e := range errs {
			fmt.Printf("- %v\n", e)
		}
	}
}

func splitCSV(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
