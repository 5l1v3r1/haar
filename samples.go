package haar

import (
	"errors"
	"fmt"
	"image"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"
)

const randomAdversaryAttempts = 50

// A SampleSource provides images for use while training
// cascade classifiers.
type SampleSource interface {
	// Positives returns positive training samples.
	Positives() []IntegralImage

	// InitialNegatives returns negative training samples
	// to use for training the first layer of a cascade.
	InitialNegatives() []IntegralImage

	// AdversarialNegatives returns negative training
	// samples which fool the existing cascade.
	AdversarialNegatives(c *Cascade) []IntegralImage
}

// LoadSampleSource creates a SampleSource from images
// on the filesystem.
//
// All of the resulting samples will be normalized to
// have a mean of 0 and a stddev of 0.
//
// All the image files must be PNGs or JPEGs.
// The positive samples must all be the same dimensions.
// However, the negative samples can have any dimensions,
// so long as they are at least as big as the positives.
func LoadSampleSource(positiveDir, negativeDir string) (SampleSource, error) {
	var pos []IntegralImage
	var neg []*DualImage

	var posWidth, posHeight int

	dirListing, err := ioutil.ReadDir(positiveDir)
	if err != nil {
		return nil, err
	}
	for _, item := range dirListing {
		if item.IsDir() || strings.HasPrefix(item.Name(), ".") {
			continue
		}
		path := filepath.Join(positiveDir, item.Name())
		img, err := readImage(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %s", path, err)
		}
		if len(pos) == 0 {
			posWidth = img.Width()
			posHeight = img.Height()
		} else if img.Width() != posWidth || img.Height() != posHeight {
			return nil, fmt.Errorf("%s: expected dimensions %dx%d got %dx%d",
				path, posWidth, posHeight, img.Width(), img.Height())
		}
		pos = append(pos, img.Window(0, 0, img.Width(), img.Height()))
	}

	if len(pos) == 0 {
		return nil, errors.New("no positive samples")
	}

	dirListing, err = ioutil.ReadDir(negativeDir)
	if err != nil {
		return nil, err
	}
	for _, item := range dirListing {
		if item.IsDir() || strings.HasPrefix(item.Name(), ".") {
			continue
		}
		path := filepath.Join(negativeDir, item.Name())
		img, err := readImage(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %s", path, err)
		}
		if img.Width() < posWidth || img.Height() < posHeight {
			return nil, fmt.Errorf("%s: dimensions %dx%d are too small", path,
				img.Width(), img.Height())
		}
		neg = append(neg, img)
	}

	if len(neg) == 0 {
		return nil, errors.New("no negative samples")
	}

	return &imageSampleSource{
		positives: pos,
		negatives: neg,
	}, nil
}

func readImage(imgPath string) (*DualImage, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil, err
	}

	bmp := ImageIntegralImage(img)
	return NewDualImage(bmp), nil
}

type imageSampleSource struct {
	positives []IntegralImage
	negatives []*DualImage
}

func (i *imageSampleSource) Positives() []IntegralImage {
	return i.positives
}

func (i *imageSampleSource) InitialNegatives() []IntegralImage {
	width, height := i.positives[0].Width(), i.positives[0].Height()

	res := make([]IntegralImage, len(i.negatives))
	for i, neg := range i.negatives {
		res[i] = randomCropping(neg, width, height)
	}
	return res
}

func (i *imageSampleSource) AdversarialNegatives(c *Cascade) []IntegralImage {
	width, height := i.positives[0].Width(), i.positives[0].Height()

	res := make([]IntegralImage, 0, len(i.negatives))

NegativeLoop:
	for _, neg := range i.negatives {
		// Attempting to pick random adversaries before
		// brute forcing adversaries will hopefully help
		// select a more diverse set of negatives in the
		// earlier stages.
		for i := 0; i < randomAdversaryAttempts; i++ {
			cropping := randomCropping(neg, width, height)
			if c.Classify(cropping) {
				res = append(res, cropping)
				continue NegativeLoop
			}
		}

		for x := 0; x <= neg.Width()-width; x++ {
			for y := 0; y <= neg.Height()-height; y++ {
				cropping := neg.Window(x, y, width, height)
				if c.Classify(cropping) {
					res = append(res, cropping)
					continue NegativeLoop
				}
			}
		}
	}

	return res
}

func randomCropping(img *DualImage, width, height int) IntegralImage {
	x := rand.Intn(img.Width() - width)
	y := rand.Intn(img.Height() - height)
	return img.Window(x, y, width, height)
}
