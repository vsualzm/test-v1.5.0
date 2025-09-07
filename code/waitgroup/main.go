package main

import (
	"fmt"
	"sync"
	"time"
)

func worker(id int, ch chan<- string, wg *sync.WaitGroup) {
	defer wg.Done()
	time.Sleep(time.Second)
	ch <- fmt.Sprintf("Worker %d selesai", id)
}

func main() {
	var wg sync.WaitGroup
	ch := make(chan string, 5)

	numWorkers := 5
	for i := 1; i <= numWorkers; i++ {
		wg.Add(1)
		go worker(i, ch, &wg)
	}

	// goroutine untuk menutup channel setelah semua worker selesai
	go func() {
		wg.Wait()
		close(ch)
	}()

	// terima semua hasil dari channel
	for msg := range ch {
		fmt.Println(msg)
	}

	fmt.Println("Semua worker sudah selesai")
}

// WaitGroup (wg) → dipakai untuk menunggu semua worker goroutine selesai.
// Channel (ch) → dipakai untuk mengirim hasil dari worker ke main goroutine.

// Keduanya dipakai bersama-sama agar:
// Worker bisa jalan concurrently.
// Hasil bisa dikumpulkan lewat channel.
// Program tahu kapan semua worker selesai (wg.Wait()), lalu channel ditutup agar for range berhenti.

// WaitGroup saja
// Kalau hanya ingin menunggu semua goroutine selesai, tanpa perlu kirim data.
// Contoh: goroutine cuma logging, atau update cache.

// Channel saja
// Kalau goroutine hanya perlu mengirim data ke main thread, tanpa peduli kapan semua selesai.
// Contoh: goroutine kirim notifikasi/event ke listener.

// WaitGroup + Channel
// Kalau butuh dua-duanya:
// menunggu semua goroutine selesai,
// dan mengumpulkan hasil pekerjaannya.
// Contoh: worker pool, concurrent data fetching, batch processing.

// Ringkasnya:
// WaitGroup = sinkronisasi (kapan semua selesai).
// Channel = komunikasi (kirim/terima data).
// Gabungan = sinkronisasi + komunikasi.
