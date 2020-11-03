package imgutil

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"io/ioutil"
	"os"
	"testing"
)

func TestT(t *testing.T) {
	tester(t, 100, 400, "test1.jpg")
	tester(t, 400, 100, "test2.jpg")
}

func tester(t *testing.T, dx int, dy int, output string) {
	src := image.NewRGBA(image.Rect(0, 0, dx, dy))
	black := color.RGBA{A: 1}

	for x := 0; x < dx; x++ {
		for y := 0; y < dy; y++ {
			src.Set(x, y, black)
		}
	}

	img := ResizeContain(src, 300, 200, color.White)
	buf := &bytes.Buffer{}
	err := jpeg.Encode(buf, img, nil)
	if err != nil {
		t.Fatal(err)
	}

	err = ioutil.WriteFile(output, buf.Bytes(), os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
}
