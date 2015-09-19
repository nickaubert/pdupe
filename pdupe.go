package main

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

import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
)

import "github.com/gographics/imagick/imagick"

const (
	HSlices = 32
	VSlices = 32
	HReSize = 128
	VReSize = 128
)

func main() {

	imagick.Initialize()
	defer imagick.Terminate()

	for _, arg := range os.Args[1:] {
		colorData := getColorData(arg)
		err := validateCD(colorData)
		if err != nil {
			fmt.Println("Error validating generated data: ", err)
			continue
		}
		outfile := arg + ".colordata"
		b := jmart(colorData)
		// b, _ := json.Marshal(colorData)
		// fmt.Println(arg, "data: ", b)
		err = ioutil.WriteFile(outfile, []byte(b), 0644)
		if err != nil {
			fmt.Println("Error writing to ", outfile, err)
			continue
		}
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

/* not entirely sure color channels work this way, but don't see anything more reliable*/
func make8bit(fullColor float64, depth uint) uint8 {
	var flatColor uint8
	o := float64(depth - 8)
	m := math.Pow(2, o)
	flatColor = uint8(fullColor / m)
	return flatColor
}

func difference(a, b uint) uint {
	if a > b {
		return a - b
	}
	return b - a
}

func getColorData(file string) []uint8 {

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	// colorData := make([]uint8, 3*HSlices*VSlices)
	var colorData []uint8

	err := mw.ReadImage(file)
	if err != nil {
		fmt.Println("Error: ", err)
		return colorData
	}

	err = mw.ResizeImage(HReSize, VReSize, imagick.FILTER_GAUSSIAN, 0.25)
	if err != nil {
		fmt.Println("resize error: ", err)
	}

	cellWidth := HReSize / HSlices
	cellHeight := VReSize / VSlices

	/*
	* colorData will hold color info for the image
	* since we're using a fixed number of cells,
	* this will be a one dimensional array of 1024 * 3 members
	 */

	redDepth := mw.GetImageChannelDepth(imagick.CHANNEL_RED)
	grnDepth := mw.GetImageChannelDepth(imagick.CHANNEL_GREEN)
	bluDepth := mw.GetImageChannelDepth(imagick.CHANNEL_BLUE)

	mc := imagick.NewMagickWand()

	// index := 0
	for vCell := 0; vCell < VSlices; vCell++ {

		for hCell := 0; hCell < HSlices; hCell++ {

			mc = mw.GetImageRegion(uint(cellWidth), uint(cellHeight), int(vCell*cellHeight), int(hCell*cellWidth))
			var redAvg, grnAvg, bluAvg float64

			if redAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_RED); err != nil {
				fmt.Println("red channel error: , ", err)
			}
			if grnAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_GREEN); err != nil {
				fmt.Println("green channel error: , ", err)
			}
			if bluAvg, _, err = mc.GetImageChannelMean(imagick.CHANNEL_BLUE); err != nil {
				fmt.Println("blue channel error: , ", err)
			}

			// make sure we're using 8 bit color
			redRnd := make8bit(redAvg, redDepth)
			grnRnd := make8bit(grnAvg, grnDepth)
			bluRnd := make8bit(bluAvg, bluDepth)

			// colorData = append(colorData, redRnd, grnRnd, bluRnd)
			colorData = append(colorData, redRnd, grnRnd, bluRnd)

			// colorData[index+0] = redRnd
			// colorData[index+1] = grnRnd
			// colorData[index+2] = bluRnd
			_ = redRnd
			_ = grnRnd
			_ = bluRnd
			// index += 3

		}
		// fmt.Println("len: ", len(colorData))
	}

	return colorData
}

func validateCD(cd []uint8) error {
	/*  we are expecting a very specific type of output so verify nothing weird happened
	 */
	gotLen := len(cd)
	xpcLen := 3 * HSlices * VSlices
	if gotLen != xpcLen {
		return fmt.Errorf("Expect array length %d, got %d\n", xpcLen, gotLen)
	}
	sum := uint8(0)
	for _, i := range cd {
		sum += i
	}
	if sum == 0 {
		return fmt.Errorf("No data in array\n")
	}
	return nil
}

func jmart(cd []uint8) string {
	/* json.Marshall wants to convert uint8 to chars
	 * I'd prefer it didn't
	 */
	jr := "["
	maxComma := len(cd) - 1
	for k, v := range cd {
		jr += fmt.Sprintf(" %d", v)
		if k < maxComma {
			jr += ","
		}
	}
	jr += " ]"
	return jr
}
