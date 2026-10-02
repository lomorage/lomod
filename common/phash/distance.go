package phash

import (
	"math"
	"github.com/steakknife/hamming"
)

// Distance represents a similatiry measure as float64 value.
type Distance float64

// Equal checks if two distances are the same, it uses the epsilon approach for comparring floats.
func (d Distance) Equal(dst Distance) bool {
	eps := math.Nextafter(1.0, 2.0) - 1.0
	if math.Abs(float64(d)-float64(dst)) > eps {
		return false
	}
	return true
}

// Hamming calculates distance between two binary hashes.
func Hamming(h1, h2 Binary) Distance {
	return Distance(hamming.Bytes(h1, h2))
}