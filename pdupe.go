package main

import (
	"fmt"
	"math"
	// "os"
)

import "github.com/gographics/imagick/imagick"

/*
 *  https://gowalker.org/github.com/gographics/imagick/imagick
 *
* Logic:
*
* get image height
* get image width
*
* divide height by 32
* divide width by 32
*
* cycle through 32x32 x,y coordinates (1024 cycles)
*         extract portion of image image
*         GetImageChannelMean of portion for CHANNEL_RED, CHANNEL_GREEN, CHANNEL_BLUE ( and CHANNELS_GRAY? )
*         add r, g, b (and gray?) mean values to array
*
* resulting array is used to compare similarity data
*
*/

func main() {

	// argsWithoutProg := os.Args[1:]

	hreSize := uint(128)
	vreSize := uint(128)

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

	err = mw.ResizeImage(hreSize, vreSize, imagick.FILTER_GAUSSIAN, 0.25)
	if err != nil {
		fmt.Println("resize error: ", err)
	}

	cellWidth := hreSize / hSlices
	cellHeight := vreSize / vSlices

	fmt.Println("width: ", mw.GetImageWidth())
	fmt.Println("height: ", mw.GetImageHeight())

	/*
			 * colorData will hold color info for the image
			 * since we're using a fixed number of cells,
		     * this will be a one dimensional array of 1024 * 3 members
	*/
	colorData := make([]uint8, 3*hSlices*hSlices)
	index := 0

	mc := imagick.NewMagickWand()

	for vCell := uint(0); vCell < vSlices; vCell++ {

		for hCell := uint(0); hCell < hSlices; hCell++ {

			mc = mw.GetImageRegion(cellWidth, cellHeight, int(vCell*cellHeight), int(hCell*cellWidth))
			var redAvg, grnAvg, bluAvg float64

			// fmt.Println("cell: ", vCell, hCell, int(hCell*hSlices))
			if redAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_RED); err != nil {
				fmt.Println("red channel error: , ", err)
			}
			if grnAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_GREEN); err != nil {
				fmt.Println("green channel error: , ", err)
			}
			if bluAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_BLUE); err != nil {
				fmt.Println("blue channel error: , ", err)
			}

			// these channels are 16 bit? knock down to 8 bit colors
			redRnd := uint8(Divide(redAvg, 256))
			grnRnd := uint8(Divide(grnAvg, 256))
			bluRnd := uint8(Divide(bluAvg, 256))

			// colorData = append(colorData, redRnd, grnRnd, bluRnd)
			colorData[index+0] = redRnd
			colorData[index+1] = grnRnd
			colorData[index+2] = bluRnd
			index += 3
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
