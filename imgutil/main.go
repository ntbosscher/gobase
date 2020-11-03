package imgutil

import (
	"github.com/nfnt/resize"
	"image"
	"image/color"
)

// ResizeContain scales the source image to fit in the width/height provided. If the image
// aspect ratio doesn't match the height/width, ResizeContain will pad the remainder with
// the padColor.
func ResizeContain(img image.Image, width int, height int, padColor color.Color) image.Image {

	bounds := img.Bounds()
	srcRatio := float32(bounds.Dx()) / float32(bounds.Dy())
	dstRatio := float32(width) / float32(height)

	// scale so the long side matches it's dimension
	if srcRatio > dstRatio {
		img = resize.Resize(uint(width), 0, img, resize.Lanczos3)
	} else {
		img = resize.Resize(0, uint(height), img, resize.Lanczos3)
	}

	bounds = img.Bounds()

	// exact match, who'd have thought!
	if bounds.Dx() == width && bounds.Dy() == height {
		return img
	}

	dstImg := image.NewRGBA(image.Rect(0, 0, width, height))

	if bounds.Dx() < width {
		// pad left and right
		imgXStart := (width - bounds.Dx()) / 2
		imgXEnd := bounds.Dx() + imgXStart

		for x := 0; x < width; x++ {
			if x < imgXStart || x > imgXEnd {
				for y := 0; y < height; y++ {
					dstImg.Set(x, y, padColor)
				}
			} else {
				imgX := bounds.Min.X + (x - imgXStart)
				for y := 0; y < height; y++ {
					dstImg.Set(x, y, img.At(imgX, y))
				}
			}
		}

		return dstImg
	}

	// pad top and bottom
	imgYStart := (height - bounds.Dy()) / 2
	imgYEnd := bounds.Dy() + imgYStart

	for y := 0; y < height; y++ {
		if y < imgYStart || y > imgYEnd {
			for x := 0; x < width; x++ {
				dstImg.Set(x, y, padColor)
			}
		} else {
			srcY := bounds.Min.Y + (y - imgYStart)
			for x := 0; x < width; x++ {
				dstImg.Set(x, y, img.At(x, srcY))
			}
		}
	}

	return dstImg
}
