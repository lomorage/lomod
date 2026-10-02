package phash

import (
	"fmt"
	"testing"
)

var distanceEqualTests = []struct {
	name string
	dst1 Distance
	dst2 Distance
	out  bool
}{
	{"same integer nums", Distance(12), Distance(12), true},
	{"different integer nums", Distance(12), Distance(33), false},
	{"different integer nums reversed", Distance(33), Distance(12), false},
	{"same decimal nums", Distance(1.23456789), Distance(1.23456789), true},
	{"different decimal nums", Distance(1.23456789), Distance(1.2345678), false},
	{"different decimal nums reversed", Distance(1.2345678), Distance(1.23456789), false},
}

func TestDistance_Equal(t *testing.T) {
	for _, tt := range distanceEqualTests {
		t.Run(tt.name, func(t *testing.T) {
			res := tt.dst1.Equal(tt.dst2)
			if res != tt.out {
				t.Errorf("got %v, want %v", res, tt.out)
			}
		})
	}
}

func ExampleDistance_Equal() {
	num1 := Distance(17.2299478)
	num2 := Distance(184.909055172)
	num3 := Distance(184.909055172)

	fmt.Println(num1.Equal(num2))
	fmt.Println(num2.Equal(num3))
	// Output:
	// false
	// true
}

var hammingTests = []struct {
	name     string
	hash1    Binary
	hash2    Binary
	distance Distance
}{
	{"two bit difference", Binary{1}, Binary{2}, 2},
	{"reverse hashes", Binary{2}, Binary{1}, 2},
	{"two bytes", Binary{1, 1}, Binary{2, 2}, 4},
	{"sample1 vs sample2", Binary{15, 131, 192, 224, 192, 252, 255, 255}, Binary{24, 60, 126, 126, 126, 126, 60, 0}, 42},
	{"sample1 vs sample3", Binary{15, 131, 192, 224, 192, 252, 255, 255}, Binary{63, 131, 192, 224, 192, 252, 255, 63}, 4},
	{"sample1 vs sample4", Binary{15, 131, 192, 224, 192, 252, 255, 255}, Binary{16, 60, 124, 126, 124, 124, 60, 24}, 38},
	{"lena vs cat", Binary{125, 121, 185, 149, 213, 197, 112, 52}, Binary{255, 255, 143, 3, 33, 65, 32, 27}, 27},
}

func TestHamming(t *testing.T) {
	for _, tt := range hammingTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if res := Hamming(tt.hash1, tt.hash2); !res.Equal(tt.distance) {
				t.Errorf("got %v, want %v", res, tt.distance)
			}
		})
	}
}

func ExampleHamming() {
	hash1 := Binary{15, 131, 192, 224, 192, 252, 255, 255}
	hash2 := Binary{24, 60, 126, 126, 126, 126, 60, 0}
	hash3 := Binary{63, 131, 192, 224, 192, 252, 255, 63}

	fmt.Println(Hamming(hash1, hash2))
	fmt.Println(Hamming(hash1, hash3))
	// Output:
	// 42
	// 4
}
