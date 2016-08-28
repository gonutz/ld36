package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/gonutz/blob"
	"github.com/gonutz/ld36/game"
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

	var info game.Info

	caveman := loadXCF("caveman")
	compile(caveman, "stand left", "caveman_stand_left")
	compile(caveman, "push left", "caveman_push_left")
	compile(caveman, "fall left", "caveman_fall_left")
	info.CavemanHitBox = scaleRect(
		extractCollisionRect(caveman.GetLayerByName("collision")), 0.25,
	)

	rocks := loadXCF("rock")
	compile(rocks, "rock", "rock")

	gates := loadXCF("gate")
	compile(gates, "a", "gate_a")
	compile(gates, "b", "gate_b")

	infoBuffer := bytes.NewBuffer(nil)
	check(json.NewEncoder(infoBuffer).Encode(info))
	check(ioutil.WriteFile(
		filepath.Join(sourcePath, "rsc", "info.json"),
		infoBuffer.Bytes(),
		0666,
	))

	// create tile sheet
	tiles := loadXCF("tiles")
	tileW, tileH := tiles.Layers[0].Bounds().Dx(), tiles.Layers[0].Bounds().Dy()
	tileCount := len(tiles.Layers)
	sheetSize := int(math.Ceil(math.Sqrt(float64(tileCount))) + 0.5)
	sheet := image.NewRGBA(image.Rect(0, 0, sheetSize*tileW, sheetSize*tileH))
	drawnTiles := 0
	for _, layer := range tiles.Layers {
		x := (drawnTiles % sheetSize) * tileW
		y := (drawnTiles / sheetSize) * tileH
		r := image.Rect(x, y, x+tileW, y+tileH)
		draw.Draw(sheet, r, layer, layer.Bounds().Min, draw.Src)
		drawnTiles++
	}
	// this is for editing the map in Tiled, it looks better with its original
	// colors
	//savePng(sheet, "tiles") // for editing in Tiled
	savePng(swapRedBlue(sheet), "tiles") // for the final game

	files, err := ioutil.ReadDir(".")
	check(err)

	output := blob.New()

	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".wav") ||
			strings.HasSuffix(f.Name(), ".png") ||
			strings.HasSuffix(f.Name(), ".tmx") ||
			strings.HasSuffix(f.Name(), ".json") {
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

func extractCollisionRect(img image.Image) game.Rectangle {
	b := img.Bounds()

	var right, bottom int
	top, left := func() (int, int) {
		for top := b.Min.Y; top < b.Max.Y; top++ {
			for left := b.Min.X; left < b.Max.X; left++ {
				if _, _, _, a := img.At(left, top).RGBA(); a > 0 {
					return top, left
				}
			}
		}
		return 0, 0
	}()

	for right = left; right < b.Max.X; right++ {
		if _, _, _, a := img.At(right, top).RGBA(); a == 0 {
			break
		}
	}
	for bottom = top; bottom < b.Max.Y; bottom++ {
		if _, _, _, a := img.At(left, bottom).RGBA(); a == 0 {
			break
		}
	}

	return game.Rectangle{
		X: left,
		Y: top,
		W: right - left,
		H: bottom - top,
	}
}

func scaleRect(r game.Rectangle, f float32) game.Rectangle {
	r.X = int(float32(r.X)*f + 0.5)
	r.Y = int(float32(r.Y)*f + 0.5)
	r.W = int(float32(r.W)*f + 0.5)
	r.H = int(float32(r.H)*f + 0.5)
	return r
}
