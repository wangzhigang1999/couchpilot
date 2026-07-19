package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateTransparentTrayIconFrames(t *testing.T) {
	source := filepath.Join("..", "..", "assets", "windows", "couchpilot-tray.svg")
	output := filepath.Join(t.TempDir(), "couchpilot.ico")
	if err := generate(source, output, ""); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 6 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
		t.Fatal("generated file is not an ICO")
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	if count != len(iconSizes) {
		t.Fatalf("ICO contains %d frames, want %d", count, len(iconSizes))
	}

	for index, expectedSize := range iconSizes {
		entryStart := 6 + index*16
		entry := data[entryStart : entryStart+16]
		actualSize := int(entry[0])
		if actualSize == 0 {
			actualSize = 256
		}
		if actualSize != expectedSize || int(entry[1]) != expectedSize%256 {
			t.Fatalf("frame %d is %dx%d, want %dx%d", index, actualSize, entry[1], expectedSize, expectedSize)
		}
		if bits := binary.LittleEndian.Uint16(entry[6:8]); bits != 32 {
			t.Fatalf("frame %d uses %d bits, want 32", index, bits)
		}

		length := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if offset < 0 || length < 0 || offset+length > len(data) {
			t.Fatalf("frame %d points outside the ICO", index)
		}
		frame, err := png.Decode(bytes.NewReader(data[offset : offset+length]))
		if err != nil {
			t.Fatalf("decode frame %d: %v", index, err)
		}
		if frame.Bounds().Dx() != expectedSize || frame.Bounds().Dy() != expectedSize {
			t.Fatalf("decoded frame %d is %v", index, frame.Bounds())
		}

		transparentPixels := 0
		opaquePixels := 0
		for y := 0; y < expectedSize; y++ {
			for x := 0; x < expectedSize; x++ {
				_, _, _, alpha := frame.At(x, y).RGBA()
				switch alpha {
				case 0:
					transparentPixels++
				case 0xffff:
					opaquePixels++
				}
			}
		}
		if transparentPixels == 0 || opaquePixels == 0 {
			t.Fatalf("frame %d does not contain both transparent and opaque pixels", index)
		}
		for _, corner := range [][2]int{{0, 0}, {expectedSize - 1, 0}, {0, expectedSize - 1}, {expectedSize - 1, expectedSize - 1}} {
			_, _, _, alpha := frame.At(corner[0], corner[1]).RGBA()
			if alpha != 0 {
				t.Fatalf("frame %d corner %v has alpha %d, want 0", index, corner, alpha)
			}
		}
	}
}
