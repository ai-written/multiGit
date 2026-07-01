//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/png"
	"os"
)

func main() {
	input := "appicon.png"
	output := "appicon.ico"

	f, err := os.Open(input)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		fmt.Println("decode:", err)
		os.Exit(1)
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	if w > 256 || h > 256 {
		w = 256
		h = 256
	}
	bw := byte(w)
	bh := byte(h)
	if w == 256 { bw = 0 }
	if h == 256 { bh = 0 }

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		fmt.Println("encode:", err)
		os.Exit(1)
	}

	out, err := os.Create(output)
	if err != nil {
		fmt.Println("create:", err)
		os.Exit(1)
	}
	defer out.Close()

	binary.Write(out, binary.LittleEndian, uint16(0))
	binary.Write(out, binary.LittleEndian, uint16(1))
	binary.Write(out, binary.LittleEndian, uint16(1))

	binary.Write(out, binary.LittleEndian, bw)
	binary.Write(out, binary.LittleEndian, bh)
	binary.Write(out, binary.LittleEndian, byte(0))
	binary.Write(out, binary.LittleEndian, byte(0))
	binary.Write(out, binary.LittleEndian, uint16(0))
	binary.Write(out, binary.LittleEndian, uint16(32))
	binary.Write(out, binary.LittleEndian, uint32(pngBuf.Len()))
	binary.Write(out, binary.LittleEndian, uint32(22))

	out.Write(pngBuf.Bytes())

	fmt.Printf("generated %s (%dx%d, %d bytes)\n", output, w, h, pngBuf.Len()+22)
}
