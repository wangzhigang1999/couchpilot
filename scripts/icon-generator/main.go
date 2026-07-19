package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

var iconSizes = []int{16, 20, 24, 32, 48, 64, 128, 256}

func main() {
	source := flag.String("source", "", "source SVG file")
	output := flag.String("output", "", "destination ICO file")
	preview := flag.String("preview", "", "optional visual preview PNG")
	flag.Parse()

	if *source == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := generate(*source, *output, *preview); err != nil {
		fmt.Fprintln(os.Stderr, "generate Windows icon:", err)
		os.Exit(1)
	}
}

func generate(source, output, preview string) error {
	svgFile, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer svgFile.Close()

	icon, err := oksvg.ReadIconStream(svgFile)
	if err != nil {
		return fmt.Errorf("parse SVG: %w", err)
	}

	frames := make([][]byte, 0, len(iconSizes))
	images := make([]*image.NRGBA, 0, len(iconSizes))
	for _, size := range iconSizes {
		canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
		icon.SetTarget(0, 0, float64(size), float64(size))
		scanner := rasterx.NewScannerGV(size, size, canvas, canvas.Bounds())
		dasher := rasterx.NewDasher(size, size, scanner)
		icon.Draw(dasher, 1)

		var frame bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&frame, canvas); err != nil {
			return fmt.Errorf("encode %dx%d frame: %w", size, size, err)
		}
		frames = append(frames, frame.Bytes())
		images = append(images, canvas)
	}

	data, err := encodeICO(iconSizes, frames)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	if preview != "" {
		if err := writePreview(preview, iconSizes, images); err != nil {
			return err
		}
	}
	return nil
}

func writePreview(output string, sizes []int, icons []*image.NRGBA) error {
	const (
		width      = 1000
		height     = 360
		margin     = 24
		largePanel = 312
		themePanel = 304
		panelGap   = 16
	)

	preview := image.NewNRGBA(image.Rect(0, 0, width, height))
	draw.Draw(preview, preview.Bounds(), &image.Uniform{C: color.NRGBA{R: 0xD8, G: 0xD5, B: 0xCF, A: 0xFF}}, image.Point{}, draw.Src)

	largeBounds := image.Rect(margin, margin, margin+largePanel, height-margin)
	drawCheckerboard(preview, largeBounds, 16)
	if len(icons) > 0 {
		largest := icons[len(icons)-1]
		draw.Draw(preview, image.Rect(margin+28, margin+28, margin+284, margin+284), largest, image.Point{}, draw.Over)
	}

	lightBounds := image.Rect(margin+largePanel+panelGap, margin, margin+largePanel+panelGap+themePanel, height-margin)
	darkBounds := lightBounds.Add(image.Pt(themePanel+panelGap, 0))
	draw.Draw(preview, lightBounds, &image.Uniform{C: color.NRGBA{R: 0xF5, G: 0xF3, B: 0xEE, A: 0xFF}}, image.Point{}, draw.Src)
	draw.Draw(preview, darkBounds, &image.Uniform{C: color.NRGBA{R: 0x20, G: 0x1D, B: 0x19, A: 0xFF}}, image.Point{}, draw.Src)

	for _, bounds := range []image.Rectangle{lightBounds, darkBounds} {
		drawNativeSizes(preview, bounds, sizes, icons)
		if len(icons) > 0 {
			drawNearest(preview, icons[0], bounds.Min.X+24, bounds.Min.Y+128, 8)
		}
		if len(icons) > 2 {
			drawNearest(preview, icons[2], bounds.Min.X+176, bounds.Min.Y+144, 4)
		}
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("create preview directory: %w", err)
	}
	file, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("create preview: %w", err)
	}
	defer file.Close()
	if err := png.Encode(file, preview); err != nil {
		return fmt.Errorf("encode preview: %w", err)
	}
	return nil
}

func drawCheckerboard(destination draw.Image, bounds image.Rectangle, cell int) {
	colors := [2]color.NRGBA{
		{R: 0xF0, G: 0xEF, B: 0xEC, A: 0xFF},
		{R: 0xCA, G: 0xC7, B: 0xC1, A: 0xFF},
	}
	for y := bounds.Min.Y; y < bounds.Max.Y; y += cell {
		for x := bounds.Min.X; x < bounds.Max.X; x += cell {
			index := ((x-bounds.Min.X)/cell + (y-bounds.Min.Y)/cell) % 2
			cellBounds := image.Rect(x, y, min(x+cell, bounds.Max.X), min(y+cell, bounds.Max.Y))
			draw.Draw(destination, cellBounds, &image.Uniform{C: colors[index]}, image.Point{}, draw.Src)
		}
	}
}

func drawNativeSizes(destination draw.Image, bounds image.Rectangle, sizes []int, icons []*image.NRGBA) {
	x := bounds.Min.X + 20
	for index, size := range sizes {
		if index >= len(icons) || size > 48 {
			continue
		}
		cell := image.Rect(x, bounds.Min.Y+32, x+48, bounds.Min.Y+80)
		target := image.Rect(
			cell.Min.X+(cell.Dx()-size)/2,
			cell.Min.Y+(cell.Dy()-size)/2,
			cell.Min.X+(cell.Dx()-size)/2+size,
			cell.Min.Y+(cell.Dy()-size)/2+size,
		)
		draw.Draw(destination, target, icons[index], image.Point{}, draw.Over)
		x += 52
	}
}

func drawNearest(destination draw.Image, source image.Image, x, y, scale int) {
	bounds := source.Bounds()
	for sourceY := bounds.Min.Y; sourceY < bounds.Max.Y; sourceY++ {
		for sourceX := bounds.Min.X; sourceX < bounds.Max.X; sourceX++ {
			pixel := source.At(sourceX, sourceY)
			target := image.Rect(
				x+(sourceX-bounds.Min.X)*scale,
				y+(sourceY-bounds.Min.Y)*scale,
				x+(sourceX-bounds.Min.X+1)*scale,
				y+(sourceY-bounds.Min.Y+1)*scale,
			)
			draw.Draw(destination, target, &image.Uniform{C: pixel}, image.Point{}, draw.Over)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func encodeICO(sizes []int, frames [][]byte) ([]byte, error) {
	if len(sizes) == 0 || len(sizes) != len(frames) || len(sizes) > 65535 {
		return nil, fmt.Errorf("invalid ICO frame set")
	}

	var output bytes.Buffer
	_ = binary.Write(&output, binary.LittleEndian, uint16(0))
	_ = binary.Write(&output, binary.LittleEndian, uint16(1))
	_ = binary.Write(&output, binary.LittleEndian, uint16(len(frames)))

	offset := uint32(6 + 16*len(frames))
	for index, frame := range frames {
		size := sizes[index]
		if size < 1 || size > 256 {
			return nil, fmt.Errorf("unsupported icon size %d", size)
		}
		dimension := byte(size)
		if size == 256 {
			dimension = 0
		}
		output.WriteByte(dimension)
		output.WriteByte(dimension)
		output.WriteByte(0) // true-color image
		output.WriteByte(0)
		_ = binary.Write(&output, binary.LittleEndian, uint16(1))
		_ = binary.Write(&output, binary.LittleEndian, uint16(32))
		_ = binary.Write(&output, binary.LittleEndian, uint32(len(frame)))
		_ = binary.Write(&output, binary.LittleEndian, offset)
		offset += uint32(len(frame))
	}
	for _, frame := range frames {
		_, _ = output.Write(frame)
	}
	return output.Bytes(), nil
}
