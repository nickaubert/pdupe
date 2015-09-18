package main

import (
	"fmt"
	"math"
	// "os"
)

import "github.com/gographics/imagick/imagick"

/*
 *  https://gowalker.org/github.com/gographics/imagick/imagick
 */

func main() {

	// argsWithoutProg := os.Args[1:]

	hSlices := uint(32)
	vSlices := uint(32)

	imagick.Initialize()
	defer imagick.Terminate()

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	err := mw.ReadImage("IMG_0696.jpg")
	if err != nil {
		fmt.Println("Error: ", err)
	}

	cellWidth := Divide(float64(mw.GetImageWidth()), hSlices)
	cellHeight := Divide(float64(mw.GetImageHeight()), vSlices)

	// fmt.Println("Image cols: ", cellWidth)
	// fmt.Println("Image rows: ", cellHeight)

	/* will need to make sure we don't overflow at boundaries since we're rounding */
	for vCell := uint(0); vCell < vSlices; vCell++ {
		ch := cellHeight
		if vCell+1 == vSlices {
			ch = mw.GetImageHeight() - (vCell * vSlices)
		}
		for hCell := uint(0); hCell < hSlices; hCell++ {
			fmt.Printf("cells: %d, %d : ", vCell, hCell)
			cw := cellWidth
			if hCell+1 == hSlices {
				cw = mw.GetImageWidth() - (hCell * hSlices)
			}
			mc := mw.GetImageRegion(cw, ch, int(vCell*vSlices), int(hCell*hSlices))
			redAvg, _, err := mc.GetImageChannelMean(imagick.CHANNEL_RED)
			grnAvg, _, err := mc.GetImageChannelMean(imagick.CHANNEL_GREEN)
			bluAvg, _, err := mc.GetImageChannelMean(imagick.CHANNEL_BLUE)
			if err != nil {
				fmt.Println("channel error: , ", err)
			}
			// these channels are 16 bit? knock down to 8 bit colors
			redRnd := Divide(redAvg, 256)
			grnRnd := Divide(grnAvg, 256)
			bluRnd := Divide(bluAvg, 256)
			fmt.Println(redRnd, grnRnd, bluRnd)
		}
		// os.Exit(0) // TESTING
	}

}

/* from https://gist.github.com/DavidVaini/10308388 */
func Divide(num float64, denom uint) (newVal uint) {
	var rounded float64
	prod := num / float64(denom)
	mod := math.Mod(prod, 1)
	if mod >= 0.5 {
		rounded = math.Ceil(prod)
	} else {
		rounded = math.Floor(prod)
	}
	return uint(rounded)
}
