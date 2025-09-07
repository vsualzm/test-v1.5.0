package main

import (
	"fmt"
	"time"
)

type Job struct {
	ID int
}

// Worker function
func worker(id int, jobs <-chan Job, results chan<- string) {
	for job := range jobs {
		// simulasi proses
		time.Sleep(500 * time.Millisecond)
		results <- fmt.Sprintf("Worker %d memproses job %d", id, job.ID)
	}
}

func main() {
	const numJobs = 5
	jobs := make(chan Job, numJobs)
	results := make(chan string, numJobs)

	// Buat 2 worker
	for w := 1; w <= 2; w++ {
		go worker(w, jobs, results)
	}

	// Masukkan job ke dalam antrian (queue)
	for j := 1; j <= numJobs; j++ {
		jobs <- Job{ID: j}
	}
	close(jobs) // Tutup channel job

	// Ambil hasil
	for i := 1; i <= numJobs; i++ {
		fmt.Println(<-results)
	}
}

// Queueing di Golang biasanya berarti mekanisme mengantrikan pekerjaan (tasks/jobs) untuk diproses secara teratur atau paralel.
// Konsep ini sering dipakai untuk:

// Job processing (misalnya kirim email, proses pembayaran, notifikasi).
// Worker pool supaya tidak semua goroutine jalan sekaligus.
// Rate limiting agar resource tidak overuse.
// Di Go, queueing bisa diimplementasikan dengan channel (karena channel bisa berperan sebagai antrian FIFO).
// Intinya
// Channel = queue (FIFO).
// Worker = goroutine yang ambil job dari queue.
// Dengan ini, kita bisa membatasi jumlah goroutine aktif dan menjaga aplikasi tetap efisien
