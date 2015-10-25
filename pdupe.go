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
	"flag"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"strings"
)

import "github.com/gographics/imagick/imagick"

// import imagick "github.com/rainycape/magick"

type imageInfo struct {
	Name  string
	Path  string
	Cdata []uint8
}

type status struct {
	RFile   string
	CSimple bool
	CPrism  bool
	CStdDev bool
	Verbose bool
}
type diffInfo struct {
	Avg    float64
	StdDev float64
}

const (
	HSlices     = 32
	VSlices     = 32
	HReSize     = 128
	VReSize     = 128
	matchThresh = 40
)

func main() {

	var s status

	reference_file := flag.String("r", "", "compare against reference file")
	comp_simple := flag.Bool("s", true, "simple comparison")
	comp_prism := flag.Bool("p", false, "prism comparison")
	comp_stddev := flag.Bool("sd", false, "stddev comparison")
	verbose := flag.Bool("v", false, "verbose")
	flag.Parse()

	s.RFile = *reference_file
	s.CSimple = *comp_simple
	s.CPrism = *comp_prism
	s.CStdDev = *comp_stddev
	s.Verbose = *verbose

	jpegs, dataFiles := checkArgs(flag.Args())

	if s.RFile != "" {
		if len(jpegs) > 0 {
			fmt.Println("-s only for use when comparing cd.gz files")
			os.Exit(1)
		}
	}

	err := scanJpegs(s, jpegs)
	if err != nil {
		fmt.Println("Error processing images:", err)
		os.Exit(1)
	}

	err = scanDataFiles(s, dataFiles)
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

func diff64(a, b float64) float64 {
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

	mw.Destroy()
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
	for _, arg := range args {
		if _, err := os.Stat(arg); os.IsNotExist(err) {
			os.Stderr.WriteString(fmt.Sprintf("No such file: %s\n", arg))
			// fmt.Printf("no such file or directory: %s\n", arg)
			// os.Exit(1)
			continue
		}
		switch {
		case strings.HasSuffix(arg, ".jpg"):
			jpegs = append(jpegs, arg)
		case strings.HasSuffix(arg, ".cd.gz"):
			cdfiles = append(cdfiles, arg)
		default:
			os.Stderr.WriteString(fmt.Sprintf("Cannot process unrecognized file type: %s\n", arg))
			// fmt.Println("Cannot process unrecognized file type", arg)
			// os.Exit(1)
			continue
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

func scanDataFiles(s status, dataFiles []string) error {

	var sImage imageInfo
	var err error
	if s.RFile != "" {
		sImage, err = scanImageData(s.RFile)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Error scanning reference image data: ", err))
			// fmt.Println("Error scanning reference image data: ", err)
			//os.Exit(1)
		}
	}

	var images []imageInfo
	for _, dataFile := range dataFiles {
		image, err := scanImageData(dataFile)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Error scanning image data: ", err))
			// fmt.Println("Error scanning image data: ", err)
			// continue
		}
		images = append(images, image)
	}

	if s.RFile != "" {
		for _, image := range images {
			showMatch(s, sImage, image)
		}
		return nil
	}

	/* compare each file to the others */
	for k, image := range images {
		if k+1 == len(images) {
			break
		}
		for _, cimage := range images[k+1:] {
			showMatch(s, image, cimage)
		}
	}

	return nil
}

func scanJpegs(s status, jpegs []string) error {

	imagick.Initialize()
	defer imagick.Terminate()

	for _, arg := range jpegs {

		colorData := getColorData(arg)

		err := validateCD(colorData)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Error validating generated data: %q", err))
			// fmt.Println("Error validating generated data: ", err)
			continue
		}

		outfile := arg + ".cd.gz"

		j, err := json.Marshal(colorData)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Error: %q", err))
			// fmt.Println("error:", err)
			continue
		}

		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		w.Write(j)
		w.Close()
		err = ioutil.WriteFile(outfile, b.Bytes(), 0644)
		if err != nil {
			// fmt.Println("Error writing to ", outfile, err)
			os.Stderr.WriteString(fmt.Sprintf("Error writing to %s: %q", outfile, err))
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

func compareColorsStdDev(s status, imageA, imageB imageInfo) (float64, string) {
	// diffSum := 0
	// var diffAvg float64
	var diffReds, diffGreens, diffBlues []float64
	/* cycle = red, green, blue */
	cycle := 0
	var cellA float64
	var cellB float64
	for k, _ := range imageA.Cdata {
		cellA = float64(imageA.Cdata[k])
		cellB = float64(imageB.Cdata[k])
		switch {
		case cycle == 0:
			diffReds = append(diffReds, cellA-cellB)
		case cycle == 1:
			diffGreens = append(diffGreens, cellA-cellB)
		case cycle == 2:
			diffBlues = append(diffBlues, cellA-cellB)
		}
		cycle++
		if cycle > 2 {
			cycle = 0
		}
	}
	var diffRed, diffGreen, diffBlue diffInfo
	diffRed.Avg = getMean(diffReds)
	diffRed.StdDev = getStdDev(diffReds, diffRed.Avg)
	diffGreen.Avg = getMean(diffGreens)
	diffGreen.StdDev = getStdDev(diffGreens, diffGreen.Avg)
	diffBlue.Avg = getMean(diffBlues)
	diffBlue.StdDev = getStdDev(diffBlues, diffBlue.Avg)
	// diffAvg = float64(diffSum) / float64(len(imageA.Cdata))
	// fmt.Println("stdavgs:", diffRed.Avg, diffGreen.Avg, diffBlue.Avg)
	// fmt.Println("stddevs:", diffRed.StdDev, diffGreen.StdDev, diffBlue.StdDev)

	// func checkem(s status, diffRed, diffGreen, diffBlue diffInfo) ( status, float64, string ) {
	// fmt.Printf("%04f, %04f, %04f\n", diffRed.Avg, diffGreen.Avg, diffBlue.Avg)
	/* trying to match */
	// matchSum := math.Abs(diffRed.Avg) + math.Abs(diffGreen.Avg) + math.Abs(diffBlue.Avg)
	if s.Verbose == true {
		fmt.Printf("avg: %04f, %04f, %04f\n", math.Abs(diffRed.Avg), math.Abs(diffGreen.Avg), math.Abs(diffBlue.Avg))
		fmt.Printf("std: %04f, %04f, %04f\n", math.Abs(diffRed.StdDev), math.Abs(diffGreen.StdDev), math.Abs(diffBlue.StdDev))
	}
	matchSum := math.Abs(diffRed.StdDev) + math.Abs(diffGreen.StdDev) + math.Abs(diffBlue.StdDev)
	return (matchSum / 3.0), ""
	// }

	// return diffRed, diffGreen, diffBlue
}

func compareColorsSimple(s status, imageA, imageB imageInfo) (float64, string) {
	diffSum := 0
	var diffAvg float64
	for k, _ := range imageA.Cdata {
		diffSum += int(difference(imageA.Cdata[k], imageB.Cdata[k]))
	}
	diffAvg = float64(diffSum) / float64(len(imageA.Cdata))
	return diffAvg, ""
}

func compareColorsPrismd(s status, imageA, imageB imageInfo) (float64, string) {
	var diffRed, diffGreen, diffBlue int
	/* cycle = red, green, blue */
	cycle := 0
	var diff int
	for k, _ := range imageA.Cdata {
		diff = int(difference(imageA.Cdata[k], imageB.Cdata[k]))
		switch {
		case cycle == 0:
			diffRed += diff
		case cycle == 1:
			diffGreen += diff
		case cycle == 2:
			diffBlue += diff
		}
		cycle++
		if cycle > 2 {
			cycle = 0
		}
	}
	dataLen := float64(len(imageA.Cdata)) / 3.0
	diffAvgRed := float64(diffRed) / dataLen
	diffAvgGreen := float64(diffGreen) / dataLen
	diffAvgBlue := float64(diffBlue) / dataLen
	if s.Verbose == true {
		fmt.Println("prism:", diffAvgRed, diffAvgGreen, diffAvgBlue)
	}
	// This is identical to simple compare
	diffAvg := (diffAvgRed + diffAvgGreen + diffAvgBlue) / 3.0
	return diffAvg, ""
}

/* https://github.com/ae6rt/golang-examples/blob/master/goeg/src/statistics_ans/statistics.go */
func getStdDev(numbers []float64, mean float64) float64 {
	total := 0.0
	for _, number := range numbers {
		total += math.Pow(number-mean, 2)
	}
	variance := total / float64(len(numbers)-1)
	return math.Sqrt(variance)
}

func getMean(numbers []float64) float64 {
	sum := 0.0
	if len(numbers) == 0 {
		return sum
	}
	for _, v := range numbers {
		sum += v
	}
	return (sum / float64(len(numbers)))
}

func scanImageData(dataFile string) (imageInfo, error) {
	var image imageInfo
	var err error
	data, err := ReadGzFile(dataFile)
	if err != nil {
		fmt.Println("Error unziping", dataFile, ":", err)
		return image, err
	}
	err = json.Unmarshal(data, &image)
	if err != nil {
		fmt.Println("Cannot process json from", dataFile, ":", err)
		return image, err
	}
	image.Path = "\"" + image.Name + "\""
	imagefile := strings.TrimSuffix(dataFile, ".cd.gz")
	_, err = os.Stat(imagefile)
	if err == nil {
		image.Path = imagefile
	}
	return image, nil
}

func showMatch(s status, imageA, imageB imageInfo) {

	if s.CSimple == true {
		diffSmpl, matched := compareColorsSimple(s, imageA, imageB)
		fmt.Printf("%04f simple %s %s %s\n", diffSmpl, matched, imageA.Path, imageB.Path)
	}

	if s.CPrism == true {
		diffPrism, matched := compareColorsPrismd(s, imageA, imageB)
		fmt.Printf("%04f prism  %s %s %s\n", diffPrism, matched, imageA.Path, imageB.Path)
	}

	if s.CStdDev == true {
		diffStdDev, matched := compareColorsStdDev(s, imageA, imageB)
		fmt.Printf("%04f stddev %s %s %s\n", diffStdDev, matched, imageA.Path, imageB.Path)
	}

	return
}
