package main

import (
	"flag"
	"fmt"
	"math"
)

type Product struct {
	Name      string
	GrossGram float64 // gram
	LengthMM  float64 // mm
	WidthMM   float64 // mm
	HeightMM  float64 // mm
}

// berat aktual (kg)
func actualWeightKg(p Product) float64 {
	return p.GrossGram / 1000.0
}

// berat volumetrik (kg)  -> (p*l*t dalam cm) / divisor
func volumetricWeightKg(p Product, divisor float64) float64 {
	lcm := p.LengthMM / 10.0
	wcm := p.WidthMM / 10.0
	hcm := p.HeightMM / 10.0
	return (lcm * wcm * hcm) / divisor
}

// chargeable = max(aktual, volumetrik)
func chargeableWeightKg(p Product, divisor float64) float64 {
	aw := actualWeightKg(p)
	vw := volumetricWeightKg(p, divisor)
	if vw > aw {
		return vw
	}
	return aw
}

func main() {

	// PENGHITUNG ONGKOS KIRIM
	// DATA SESUAI DENGAN TABLE SOAL
	qtyA := flag.Int("A", 1, "quantity Produk A")
	qtyB := flag.Int("B", 1, "quantity Produk B")
	rate := flag.Int("rate", 10000, "tarif per kg (Rp)")
	divisor := flag.Float64("div", 6000, "divisor volumetrik (umum domestik: 6000)")
	flag.Parse()

	products := map[string]Product{
		"A": {Name: "Produk A", GrossGram: 30, LengthMM: 115, WidthMM: 85, HeightMM: 25},
		"B": {Name: "Produk B", GrossGram: 28000, LengthMM: 1290, WidthMM: 300, HeightMM: 625},
	}
	qtys := map[string]int{
		"A": *qtyA,
		"B": *qtyB,
	}

	calcCost := func(totalKg float64) int {
		kgBill := math.Ceil(totalKg)
		return int(kgBill) * (*rate)
	}

	var totalKg float64
	fmt.Println("Rincian per-item (chargeable weight):")
	for code, p := range products {
		q := qtys[code]
		if q <= 0 {
			continue
		}
		cw := chargeableWeightKg(p, *divisor)
		fmt.Printf("- %s (qty %d): actual=%.3f kg, volumetrik=%.3f kg, chargeable=%.3f kg\n",
			p.Name, q, actualWeightKg(p), volumetricWeightKg(p, *divisor), cw)
		totalKg += cw * float64(q)
	}
	cost := calcCost(totalKg)

	fmt.Printf("\nTotal chargeable weight: %.3f kg\n", totalKg)
	fmt.Printf("Tarif: Rp %d/kg (dibulatkan ke atas)\n", *rate)
	fmt.Printf("Ongkos kirim: Rp %d\n", cost)
}

// CARA MENJALANKAN

// 1. go run main.go                 # default: A=1, B=1, rate=10000, divisor=6000
// 2. go run main.go -A=5 -B=1       # contoh: Produk A qty 5, Produk B qty 1
// 3. go run main.go -rate=12000     # jika tarif berubah
// 4. go run main.go -div=5000       # jika divisor volumetrik berbeda

// menggunakan flag agar bisa di ubah2 dan untuk testing bisa di satukan contohnya seperti ini
// contoh: go run main.go -A=5 -B=1 -rate=12000 -div=5000
