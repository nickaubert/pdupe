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
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strings"
)

import "github.com/gographics/imagick/imagick"

type imageInfo struct {
	Name  string
	Cdata []uint8
}

const (
	HSlices = 32
	VSlices = 32
	HReSize = 128
	VReSize = 128
)

func main() {

	jpegs, dataFiles := checkArgs(os.Args)

	err := scanJpegs(jpegs)
	if err != nil {
		fmt.Println("Error processing images:", err)
		os.Exit(1)
	}

	err = scanDataFiles(dataFiles)
	if err != nil {
		fmt.Println("Error processing data files:", err)
		os.Exit(1)
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

func difference(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func getColorData(file string) imageInfo {

	var colorData imageInfo
	colorData.Name = file

	fmt.Println("reading color data for ", file)

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	err := mw.ReadImage(file)
	if err != nil {
		fmt.Println("Error: ", err)
		return colorData
	}

	/* hm, maybe chop original image into pieces first, then resize those */
	err = mw.ResizeImage(HReSize, VReSize, imagick.FILTER_GAUSSIAN, 0.1)
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

			/* make sure we're using 8 bit color */
			redRnd := make8bit(redAvg, redDepth)
			grnRnd := make8bit(grnAvg, grnDepth)
			bluRnd := make8bit(bluAvg, bluDepth)

			colorData.Cdata = append(colorData.Cdata, redRnd, grnRnd, bluRnd)

		}
	}

	return colorData
}

func validateCD(cd imageInfo) error {
	/*  we are expecting a very specific type of output so verify nothing weird happened
	 */
	gotLen := len(cd.Cdata)
	xpcLen := 3 * HSlices * VSlices
	if gotLen != xpcLen {
		return fmt.Errorf("Expect array length %d, got %d\n", xpcLen, gotLen)
	}
	sum := uint8(0)
	for _, i := range cd.Cdata {
		sum += i
	}
	if sum == 0 {
		return fmt.Errorf("No data in array\n")
	}
	return nil
}

func checkArgs(args []string) ([]string, []string) {
	var jpegs []string
	var cdfiles []string
	for _, arg := range args[1:] {
		if _, err := os.Stat(arg); os.IsNotExist(err) {
			fmt.Printf("no such file or directory: %s", arg)
			os.Exit(1)
		}
		switch {
		case strings.HasSuffix(arg, ".jpg"):
			jpegs = append(jpegs, arg)
		case strings.HasSuffix(arg, ".cd.gz"):
			cdfiles = append(cdfiles, arg)
		default:
			fmt.Println("Cannot process unrecognized file type", arg)
			os.Exit(1)
		}
	}
	if len(jpegs) > 0 {
		if len(cdfiles) > 0 {
			fmt.Println("Cannot process photos and data files at the same time")
			os.Exit(1)
		}
		return jpegs, cdfiles
	}
	if len(cdfiles) == 0 {
		fmt.Println("Must select files to process")
		os.Exit(1)
	}
	return jpegs, cdfiles
}

func scanDataFiles(dataFiles []string) error {

	var images []imageInfo
	for _, dataFile := range dataFiles {
		// fmt.Println("process data file", dataFile)
		data, err := ReadGzFile(dataFile)
		if err != nil {
			fmt.Println("Error unziping", dataFile, ":", err)
		}
		var image imageInfo
		err = json.Unmarshal(data, &image)
		if err != nil {
			fmt.Println("Cannot process json from", dataFile, ":", err)
			continue
		}
		images = append(images, image)
	}

	/* compare each file to the others */
	for k, image := range images {
		if k+1 == len(images) {
			break
		}
		for _, cimage := range images[k+1:] {
			diffAvg := compareColors(image, cimage)
			fmt.Println(image.Name, "->", cimage.Name, "=", diffAvg)
		}
	}

	return nil
}

func scanJpegs(jpegs []string) error {

	imagick.Initialize()
	defer imagick.Terminate()

	for _, arg := range jpegs {

		colorData := getColorData(arg)

		err := validateCD(colorData)
		if err != nil {
			fmt.Println("Error validating generated data: ", err)
			continue
		}

		outfile := arg + ".cd.gz"

		j, err := json.Marshal(colorData)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}

		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(j)
		w.Close()
		err = ioutil.WriteFile(outfile, b.Bytes(), 0644)
		if err != nil {
			fmt.Println("Error writing to ", outfile, err)
			continue
		}

	}

	return nil
}

/* http://stackoverflow.com/questions/16890648/how-can-i-use-golangs-compress-gzip-package-to-gzip-a-file */
func ReadGzFile(filename string) ([]byte, error) {
	fi, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fi.Close()

	fz, err := gzip.NewReader(fi)
	if err != nil {
		return nil, err
	}
	defer fz.Close()

	s, err := ioutil.ReadAll(fz)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func compareColors(imageA, imageB imageInfo) float64 {
	diffSum := 0
	var diffAvg float64
	for k, _ := range imageA.Cdata {
		diffSum += int(difference(imageA.Cdata[k], imageB.Cdata[k]))
	}
	diffAvg = float64(diffSum) / float64(len(imageA.Cdata))
	return diffAvg
}
