package main

import (
	"fmt"
	"log"
	"os"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	// Optionally register camera makenote data parsing - currently Nikon and
	// Canon are supported.
	exif.RegisterParsers(mknote.All...)

	x, err := exif.Decode(f)
	if err != nil {
		log.Println("invalid EXIF data")
		log.Fatal(err)
		return
	}

	// Two convenience functions exist for date/time taken and GPS coords:
	tm, _ := x.DateTime()
	fmt.Println("Taken: ", tm)
	if tz, _ := x.TimeZone(); tz != nil {
		fmt.Println("no timezone info")
	} else {
		fmt.Println(tz)
	}

	lat, long, _ := x.LatLong()
	fmt.Println("lat, long: ", lat, ", ", long)

	fmt.Println(x)
}
