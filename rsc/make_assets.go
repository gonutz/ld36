package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/gonutz/blob"
	"github.com/gonutz/xcf"
	"github.com/nfnt/resize"
)

var (
	sourcePath = filepath.Join(
		os.Getenv("GOPATH"),
		"src",
		"github.com",
		"gonutz",
		"ld36",
	)
)

func main() {
	var outputPath string

	if len(os.Args) == 1 {
		outputPath = filepath.Join(sourcePath, "bin", "blob")
	} else if len(os.Args) == 2 {
		outputPath = os.Args[1]
	} else {
		panic("must give one parameter: the output blob file path")
	}

	caveman := loadXCF("caveman")
	compile(caveman, "stand left", "caveman_stand_left")
	compile(caveman, "push left", "caveman_push_left")

	rocks := loadXCF("rock")
	compile(rocks, "rock", "rock")

	gates := loadXCF("gate")
	compile(gates, "a", "gate_a")
	compile(gates, "b", "gate_b")

	files, err := ioutil.ReadDir(".")
	check(err)

	output := blob.New()

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".wav") ||
			strings.HasSuffix(f.Name(), ".png") {
			data, err := ioutil.ReadFile(f.Name())
			check(err)
			output.Append(f.Name(), data)
		}
	}

	file, err := os.Create(outputPath)
	check(err)
	defer file.Close()
	check(output.Write(file))
}

func compile(canvas xcf.Canvas, layerName, outputName string) {
	layer := canvas.GetLayerByName(layerName)
	savePng(
		swapRedBlue(scaleImage(makeTransparentAreasBlack(layer), 0.25)),
		outputName,
	)
}

func loadXCF(name string) xcf.Canvas {
	path := filepath.Join(sourcePath, "rsc", name+".xcf")
	canvas, err := xcf.LoadFromFile(path)
	check(err)
	return canvas
}

func savePng(img image.Image, name string) {
	path := filepath.Join(sourcePath, "rsc", name+".png")
	file, err := os.Create(path)
	check(err)
	defer file.Close()
	check(png.Encode(file, img))
}

func scaleImage(img image.Image, f float64) image.Image {
	return resize.Resize(
		uint(0.5+float64(img.Bounds().Dx())*f),
		uint(0.5+float64(img.Bounds().Dy())*f),
		img,
		resize.Bicubic,
	)
}

func swapRedBlue(img image.Image) image.Image {
	b := img.Bounds()
	swapped := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			swapped.Set(x, y, flipRB{img.At(x, y)})
		}
	}
	return swapped
}

type flipRB struct {
	color.Color
}

func (c flipRB) RGBA() (r, g, b, a uint32) {
	b, g, r, a = c.Color.RGBA()
	return
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func makeTransparentAreasBlack(original image.Image) image.Image {
	b := original.Bounds()
	img := image.NewRGBA(b)
	draw.Draw(img, b, original, image.ZP, draw.Src)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a == 0 {
				img.Set(x, y, color.RGBA{})
			}
		}
	}
	return img
}
