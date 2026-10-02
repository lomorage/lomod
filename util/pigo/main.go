package main

import (
	"fmt"
	"image"
	"image/jpeg"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"

	pigo "github.com/esimov/pigo/core"
	"github.com/pkg/errors"
)

var (
	faceFile     = "facefinder"
	pupFile      = "puploc"
	eyesCascade  = []string{"lp46", "lp44", "lp42", "lp38", "lp312"}
	mouthCascade = []string{"lp93", "lp84", "lp82", "lp81"}

	classifier       *pigo.Pigo
	puplocClassifier *pigo.PuplocCascade
	eyesFlp          []*pigo.PuplocCascade
	mouthFlp         []*pigo.PuplocCascade
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: [/path/to/image]")
	}
	cascadeFile, err := ioutil.ReadFile(faceFile)
	if err != nil {
		log.Fatalf("Error reading the cascade file: %v", err)
	}

	puplocCascade, err := ioutil.ReadFile(pupFile)
	if err != nil {
		log.Fatalf("Error reading the puploc cascade file: %s", err)
	}

	pg := pigo.NewPigo()
	// Unpack the binary file. This will return the number of cascade trees,
	// the tree depth, the threshold and the prediction from tree's leaf nodes.
	classifier, err = pg.Unpack(cascadeFile)
	if err != nil {
		log.Fatalf("Error reading the cascade file: %s", err)
	}

	pl := pigo.NewPuplocCascade()
	puplocClassifier, err = pl.UnpackCascade(puplocCascade)
	if err != nil {
		log.Fatalf("Error unpacking the puploc cascade file: %s", err)
	}

	// eyes
	for _, fn := range eyesCascade {
		f, err := ioutil.ReadFile(filepath.Join(filepath.Join(".", "lps"), fn))
		if err != nil {
			log.Fatalf("failed to read %s: %s", fn, err)
		}
		flpc, err := pl.UnpackCascade(f)
		if err != nil {
			log.Fatalf("failed to unpack %s: %s", fn, err)
		}
		eyesFlp = append(eyesFlp, flpc)
	}

	// mouth
	for _, fn := range mouthCascade {
		f, err := ioutil.ReadFile(filepath.Join(filepath.Join(".", "lps"), fn))
		if err != nil {
			log.Fatalf("failed to read %s: %s", fn, err)
		}
		flpc, err := pl.UnpackCascade(f)
		if err != nil {
			log.Fatalf("failed to unpack %s: %s", fn, err)
		}
		mouthFlp = append(mouthFlp, flpc)
	}

	if err := process(os.Args[1]); err != nil {
		log.Fatal(err)
	}
}

func process(filename string) error {
	stat, err := os.Stat(filename)
	if err != nil {
		return err
	}
	if !stat.IsDir() {
		return extractFace(filename)
	}
	files, err := ioutil.ReadDir(filename)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := process(filepath.Join(filename, f.Name())); err != nil {
			return err
		}
	}
	return nil
}

func extractFace(filename string) error {
	src, err := pigo.GetImage(filename)
	if err != nil {
		return errors.Errorf("Cannot open the image file: %v", err)
	}

	pixels := pigo.RgbToGrayscale(src)
	cols, rows := src.Bounds().Max.X, src.Bounds().Max.Y

	imageParams := pigo.ImageParams{
		Pixels: pixels,
		Rows:   rows,
		Cols:   cols,
		Dim:    cols,
	}
	cParams := pigo.CascadeParams{
		MinSize:     20,
		MaxSize:     1000,
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,

		ImageParams: imageParams,
	}

	var dets []pigo.Detection
	// cascade rotation angle. 0.0 is 0 radians and 1.0 is 2*pi radians
	for _, angle := range []float64{0.0, 0.25, 0.5, 0.75} {
		// Run the classifier over the obtained leaf nodes and return the detection results.
		// The result contains quadruplets representing the row, column, scale and detection score.
		dets = classifier.RunCascade(cParams, angle)

		// Calculate the intersection over union (IoU) of two clusters.
		dets = classifier.ClusterDetections(dets, 0.41)

		dets = filter(dets, cols, rows)

		if len(dets) != 0 {
			break
		}
	}
	var qThresh float32 = 5.0
	for i, face := range dets {
		x0 := face.Col - face.Scale/2
		y0 := face.Row - face.Scale/2
		x1 := face.Col + face.Scale/2
		y1 := face.Row + face.Scale/2
		if x0 < 0 || y0 < 0 || x1 >= cols || y1 >= rows {
			log.Printf("%s detected invalid face %+v\n", filename, face)
			continue
		}

		if face.Q < qThresh {
			log.Printf("%s detected face %+v less than thresold %f\n", filename, face, qThresh)
			continue
		}

		findEyes := locatePupil(face, puplocClassifier, imageParams)
		if !findEyes {
			log.Printf("%s detected face %+v unable locate eyes\n", filename, face)
			continue
		}

		subImage := src.SubImage(image.Rect(x0, y0, x1, y1))

		f, err := os.Create(fmt.Sprintf("%s_face%d_%f.jpg", filename, i, face.Q))
		if err != nil {
			return err
		}
		defer f.Close()
		err = jpeg.Encode(f, subImage, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

func locatePupil(face pigo.Detection, puplocClassifier *pigo.PuplocCascade, imageParams pigo.ImageParams) bool {
	// left eye
	puploc := pigo.Puploc{
		Row:      face.Row - int(0.085*float32(face.Scale)),
		Col:      face.Col - int(0.185*float32(face.Scale)),
		Scale:    float32(face.Scale) * 0.4,
		Perturbs: 63,
	}
	detLeft := puplocClassifier.RunDetector(puploc, imageParams, 0.0, false)
	if detLeft.Row == 0 || detLeft.Col == 0 {
		fmt.Println("unable to find left eye")
		return false
	}

	// right eye
	puploc = pigo.Puploc{
		Row:      face.Row - int(0.085*float32(face.Scale)),
		Col:      face.Col + int(0.185*float32(face.Scale)),
		Scale:    float32(face.Scale) * 0.4,
		Perturbs: 63,
	}

	detRight := puplocClassifier.RunDetector(puploc, imageParams, 0.0, false)
	if detRight.Row == 0 || detRight.Col == 0 {
		fmt.Println("unable to find right eye")
		return false
	}

	// skip landmark detection because it doesn't work
	/*
		// try eyes
		found := false
		for _, lc := range eyesFlp {
			flp := lc.GetLandmarkPoint(detLeft, detRight, imageParams, 63, false)
			if flp.Row > 0 && flp.Col > 0 {
				found = true
				break
			}
			flp = lc.GetLandmarkPoint(detLeft, detRight, imageParams, 63, true)
			if flp.Row > 0 && flp.Col > 0 {
				found = true
				break
			}
		}
		if !found {
			fmt.Println("unable to find eye landmark points")
			return false
		}

		found = false
		for _, lc := range mouthFlp {
			flp := lc.GetLandmarkPoint(detLeft, detRight, imageParams, 63, false)
			if flp.Row > 0 && flp.Col > 0 {
				found = true
				break
			}
			flp = lc.GetLandmarkPoint(detLeft, detRight, imageParams, 63, true)
			if flp.Row > 0 && flp.Col > 0 {
				found = true
				break
			}
		}
		if !found {
			fmt.Println("unable to find mouse landmark points")
			return false
		}
	*/
	fmt.Printf("find eyes (%d,%d), (%d,%d) for face %+v\n", detLeft.Row, detLeft.Col, detRight.Row, detRight.Col, face)
	return true
}

// qualityThreshold returns the scale adjusted quality score threshold.
func qualityThreshold(scale int) (score float32) {
	score = 9.0

	// Smaller faces require higher quality.
	switch {
	case scale < 26:
		score += 26.0
	case scale < 32:
		score += 16.0
	case scale < 40:
		score += 11.0
	case scale < 50:
		score += 9.0
	case scale < 80:
		score += 6.0
	case scale < 110:
		score += 2.0
	}

	return score
}

type Area struct {
	X float32 `json:"x,omitempty"`
	Y float32 `json:"y,omitempty"`
	W float32 `json:"w,omitempty"`
	H float32 `json:"h,omitempty"`
}

// Surface returns the surface area.
func (a Area) Surface() float64 {
	return float64(a.W * a.H)
}

// SurfaceRatio returns the surface ratio.
func (a Area) SurfaceRatio(area float64) float64 {
	if area <= 0 {
		return 0
	}

	if s := a.Surface(); s <= 0 {
		return 0
	} else if area > s {
		return s / area
	} else {
		return area / s
	}
}

// Top returns the top Y coordinate as float64.
func (a Area) Top() float64 {
	return float64(a.Y)
}

// Left returns the left X coordinate as float64.
func (a Area) Left() float64 {
	return float64(a.X)
}

// Right returns the right X coordinate as float64.
func (a Area) Right() float64 {
	return float64(a.X + a.W)
}

// Bottom returns the bottom Y coordinate as float64.
func (a Area) Bottom() float64 {
	return float64(a.Y + a.H)
}

// Overlap calculates the overlap of two areas.
func (a Area) Overlap(other Area) (x, y float64) {
	x = math.Max(0, math.Min(a.Right(), other.Right())-math.Max(a.Left(), other.Left()))
	y = math.Max(0, math.Min(a.Bottom(), other.Bottom())-math.Max(a.Top(), other.Top()))

	return x, y
}

// OverlapArea calculates the overlap area of two areas.
func (a Area) OverlapArea(other Area) (area float64) {
	x, y := a.Overlap(other)

	return x * y
}

// OverlapPercent calculates the overlap ratio of two areas in percent.
func (a Area) OverlapPercent(other Area) int {
	return int(math.Round(other.SurfaceRatio(a.OverlapArea(other)) * 100))
}

func newArea(f pigo.Detection, imgCols, imgRows int) Area {
	x := float32(f.Col-f.Scale/2) / float32(imgCols)
	y := float32(f.Row-f.Scale/2) / float32(imgRows)

	return Area{
		X: x,
		Y: y,
		W: float32(f.Scale) / float32(imgCols),
		H: float32(f.Scale) / float32(imgRows),
	}
}

func filter(faces []pigo.Detection, imgCols, imgRows int) []pigo.Detection {
	// Sort results by size.
	sort.Slice(faces, func(i, j int) bool {
		return faces[i].Scale > faces[j].Scale
	})

	results := []pigo.Detection{}
	for i, face := range faces {
		if i == 0 {
			results = append(results, face)
			continue
		}
		currArea := newArea(face, imgCols, imgRows)
		found := false
		for _, ret := range results {
			if newArea(ret, imgCols, imgRows).OverlapPercent(currArea) > 41 {
				found = true
				break
			}
		}
		if !found {
			results = append(results, face)
		}
	}
	return results
}
