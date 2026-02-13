//go:build darwin

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"
)

var (
	trayIconCacheMu sync.Mutex
	trayIconCache   = map[int][]byte{}
)

func trayIconBytes(count int) []byte {
	if count < 0 {
		count = 0
	}
	if count > 99 {
		count = 99
	}

	trayIconCacheMu.Lock()
	defer trayIconCacheMu.Unlock()

	if b, ok := trayIconCache[count]; ok {
		return b
	}

	img := image.NewNRGBA(image.Rect(0, 0, 22, 22))
	drawLock(img)
	if count > 0 {
		drawBadge(img, count)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	out := buf.Bytes()
	trayIconCache[count] = out
	return out
}

func drawLock(img *image.NRGBA) {
	black := color.NRGBA{R: 17, G: 17, B: 17, A: 255}

	// body
	for y := 10; y <= 18; y++ {
		for x := 6; x <= 15; x++ {
			img.SetNRGBA(x, y, black)
		}
	}
	// shackle
	for y := 5; y <= 10; y++ {
		img.SetNRGBA(7, y, black)
		img.SetNRGBA(14, y, black)
	}
	for x := 8; x <= 13; x++ {
		img.SetNRGBA(x, 4, black)
	}
}

func drawBadge(img *image.NRGBA, count int) {
	red := color.NRGBA{R: 230, G: 60, B: 60, A: 255}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}

	cx, cy, r := 17, 6, 6
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= r*r {
				if image.Pt(x, y).In(img.Bounds()) {
					img.SetNRGBA(x, y, red)
				}
			}
		}
	}

	digits := twoDigit(count)
	scale := 1
	if len(digits) == 1 {
		scale = 2
	}

	charW := 3 * scale
	charH := 5 * scale
	gap := scale
	totalW := len(digits)*charW + (len(digits)-1)*gap
	startX := cx - totalW/2
	startY := cy - charH/2

	for i, ch := range digits {
		drawDigitScaled(img, ch, startX+i*(charW+gap), startY, scale, white)
	}
}

func twoDigit(n int) string {
	if n < 10 {
		return string('0' + rune(n))
	}
	tens := n / 10
	ones := n % 10
	return string('0'+rune(tens)) + string('0'+rune(ones))
}

var digitBitmap = map[rune][5]byte{
	'0': {0b111, 0b101, 0b101, 0b101, 0b111},
	'1': {0b010, 0b110, 0b010, 0b010, 0b111},
	'2': {0b111, 0b001, 0b111, 0b100, 0b111},
	'3': {0b111, 0b001, 0b111, 0b001, 0b111},
	'4': {0b101, 0b101, 0b111, 0b001, 0b001},
	'5': {0b111, 0b100, 0b111, 0b001, 0b111},
	'6': {0b111, 0b100, 0b111, 0b101, 0b111},
	'7': {0b111, 0b001, 0b010, 0b010, 0b010},
	'8': {0b111, 0b101, 0b111, 0b101, 0b111},
	'9': {0b111, 0b101, 0b111, 0b001, 0b111},
}

func drawDigitScaled(img *image.NRGBA, digit rune, x, y, scale int, c color.NRGBA) {
	bm, ok := digitBitmap[digit]
	if !ok {
		return
	}
	if scale < 1 {
		scale = 1
	}
	for row := 0; row < len(bm); row++ {
		for col := 0; col < 3; col++ {
			if bm[row]&(1<<(2-col)) == 0 {
				continue
			}
			px0 := x + col*scale
			py0 := y + row*scale
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					px := px0 + sx
					py := py0 + sy
					if image.Pt(px, py).In(img.Bounds()) {
						img.SetNRGBA(px, py, c)
					}
				}
			}
		}
	}
}
