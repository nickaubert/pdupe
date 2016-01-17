package main

/*
 *  https://gowalker.org/github.com/gographics/imagick/imagick
 *  crashing a lot, maybe switch to https://golang.org/pkg/image/
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
	"runtime"
	"strings"
)

import "github.com/gographics/imagick/imagick"

// import imagick "github.com/rainycape/magick"

type imageInfo struct {
	Size  int64
	Name  string
	Path  string
	Cdata []uint8
}

type status struct {
	Comp   int // 0 = simple, 1 = prism, 2 = stddev
	Thresh int
	MaxPrc int
	// CSimple bool
	// CPrism  bool
	// CStdDev bool
	GDOnly  bool
	Verbose bool
	OvrWr   bool
}
type diffInfo struct {
	Avg    float64
	StdDev float64
}

const (
	HSlices      = 32
	VSlices      = 32
	HReSize      = 128
	VReSize      = 128
	simpleThresh = 10
	prismThresh  = 10
	stdDevThresh = 15
)

func main() {

	var s status

	reference_file := flag.String("r", "", "compare against reference file")
	comp_type := flag.String("c", "s", "s=simple, p=prism, d=stddev")
	threshold := flag.Int("t", 10, "compare type (10=default for simple)")
	overwrite := flag.Bool("o", false, "overwrite cd.gz files")
	dataonly := flag.Bool("d", false, "generate .cd.gz files only (dont compare)")
	verbose := flag.Bool("v", false, "verbose")
	maxprocs := flag.Int("p", runtime.GOMAXPROCS(0), "max cpu procs")
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Println("Error: must give file path as argument")
	}

	s.Verbose = *verbose
	s.Comp = checkCompType(*comp_type)
	s.OvrWr = *overwrite
	s.Thresh = *threshold
	s.MaxPrc = *maxprocs
	s.GDOnly = *dataonly

	var refImgsData []imageInfo
	if len(*reference_file) > 0 {

		refPath := make([]string, 1)
		refPath[0] = *reference_file
		rimages, rDataFiles := checkFiles(refPath)

		newRJpegs := scanJpegs(s, rimages)

		rDataFiles = append(rDataFiles, newRJpegs...)
		rDataFiles = dedupe(rDataFiles)

		refImgsData = scanDataFiles(s, rDataFiles)

	}

	images, dataFiles := checkFiles(flag.Args())

	newDataFiles := scanJpegs(s, images)

	dataFiles = append(dataFiles, newDataFiles...)
	dataFiles = dedupe(dataFiles)

	imgsData := scanDataFiles(s, dataFiles)

	if s.GDOnly != true {
		compareImages(s, imgsData, refImgsData)
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

/* not entirely sure color channels work this way, but don't see anything more reliable */
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

func getColorData(file string) (imageInfo, error) {

	var colorData imageInfo
	colorData.Name = file

	fmt.Println("reading color data for ", file)

	fi, err := os.Stat(file)
	if err != nil {
		return colorData, err
	}
	colorData.Size = fi.Size()

	imagick.Initialize()
	defer imagick.Terminate()

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	err = mw.ReadImage(file)
	if err != nil {
		return colorData, err
	}

	orientation := mw.GetImageOrientation()
	if orientation > 1 {
		/* http://sylvana.net/jpegcrop/exif_orientation.html */
		pw := imagick.NewPixelWand()
		switch orientation {
		case 2:
			err = mw.FlopImage()
		case 3:
			err = mw.RotateImage(pw, 180.0)
		case 4:
			err = mw.FlipImage()
		case 5:
			if err := mw.FlipImage(); err != nil {
				fmt.Println("Error: ", err)
			}
			err = mw.RotateImage(pw, 90.0)
		case 6:
			err = mw.RotateImage(pw, 90.0)
		case 7:
			if err := mw.FlipImage(); err != nil {
				fmt.Println("Error: ", err)
			}
			err = mw.RotateImage(pw, 270.0)
		case 8:
			err = mw.RotateImage(pw, 270.0)
		}
		if err != nil {
			fmt.Println("Error: ", err)
		}
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

	// mw.Destroy()
	// imagick.Terminate()
	return colorData, nil
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

func checkFiles(args []string) ([]string, []string) {

	var images []string
	var cdfiles []string

	for _, arg := range args {
		fi, err := os.Stat(arg)
		if os.IsNotExist(err) {
			os.Stderr.WriteString(fmt.Sprintf("No such file: %s\n", arg))
			continue
		}
		if fi.IsDir() == true {
			if strings.HasPrefix(fi.Name(), ".") == true {
				continue
			}
			newJpegs, newCdFiles := scanRecusive(arg)
			images = append(images, newJpegs...)
			cdfiles = append(cdfiles, newCdFiles...)
			continue
		}
		if fi.Mode().IsRegular() == true {
			switch {
			case strings.HasSuffix(arg, ".jpg"):
				images = append(images, arg)
			case strings.HasSuffix(arg, ".png"):
				images = append(images, arg)
			case strings.HasSuffix(arg, ".cd.gz"):
				cdfiles = append(cdfiles, arg)
			default:
				os.Stderr.WriteString(fmt.Sprintf("Cannot process unrecognized file type: %s\n", arg))
			}
			continue
		}
		os.Stderr.WriteString(fmt.Sprintf("Skipping non-regular file: %s\n", arg))
	}
	images = dedupe(images)
	for _, img := range images {
		cdfile := img + ".cd.gz"
		if checkFile(cdfile) == true {
			cdfiles = append(cdfiles, cdfile)
		}
	}
	return images, cdfiles
}

func scanDataFiles(s status, dataFiles []string) []imageInfo {

	var images []imageInfo
	/* dont compare image data */
	if s.GDOnly == true {
		return images
	}

	for _, dataFile := range dataFiles {
		if dataFile == "" {
			continue
		}
		image, err := scanImageData(dataFile)
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("Error scanning image data: ", err))
			continue
		}
		images = append(images, image)
	}

	return images

}

func compareImages(s status, images, refImages []imageInfo) {

	/* compare images against reference images */
	if len(refImages) > 0 {
		for _, rImage := range refImages {
			for _, image := range images {
				showMatch(s, rImage, image)
			}
		}
		return
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

	return
}

func scanJpegs(s status, images []string) []string {

	jdx := 0

	chData := make(chan string)

	/* fill queue with number of processors */
	for i := 0; i < s.MaxPrc; i++ {
		if i == len(images) {
			break
		}
		go processJpeg(chData, images[jdx], s)
		jdx++
	}

	var newDataFiles []string

	for ; jdx < len(images); jdx++ {
		newDataFiles = append(newDataFiles, <-chData)
		go processJpeg(chData, images[jdx], s)
	}

	/* drain queue */
	for len(newDataFiles) < len(images) {
		newDataFiles = append(newDataFiles, <-chData)
	}

	return newDataFiles
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

func compareColorsStdDev(s status, imageA, imageB imageInfo) float64 {

	/* cycle = red, green, blue */
	var diffReds, diffGreens, diffBlues []float64
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

	if s.Verbose == true {
		fmt.Printf("avg: %04f, %04f, %04f\n", math.Abs(diffRed.Avg), math.Abs(diffGreen.Avg), math.Abs(diffBlue.Avg))
		fmt.Printf("std: %04f, %04f, %04f\n", math.Abs(diffRed.StdDev), math.Abs(diffGreen.StdDev), math.Abs(diffBlue.StdDev))
	}
	matchSum := math.Abs(diffRed.StdDev) + math.Abs(diffGreen.StdDev) + math.Abs(diffBlue.StdDev)
	return (matchSum / 3.0)

}

func compareColorsSimple(s status, imageA, imageB imageInfo) float64 {
	diffSum := 0
	var diffAvg float64
	for k, _ := range imageA.Cdata {
		diffSum += int(difference(imageA.Cdata[k], imageB.Cdata[k]))
	}
	diffAvg = float64(diffSum) / float64(len(imageA.Cdata))
	return diffAvg
}

func compareColorsPrismd(s status, imageA, imageB imageInfo) float64 {

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

	/* this is identical to simple compare */
	diffAvg := (diffAvgRed + diffAvgGreen + diffAvgBlue) / 3.0
	return diffAvg
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
	if checkFile(imagefile) {
		image.Path = imagefile
	}
	return image, nil
}

func showMatch(s status, imageA, imageB imageInfo) {

	/*
		if s.CSimple == true {
			diffSmpl := compareColorsSimple(s, imageA, imageB)
			matched = ckThresh(diffSmpl, simpleThresh)
			if s.Verbose != true {
				fmt.Printf("%s %s\n", imageA.Path, imageB.Path)
			} else {
				fmt.Printf("%04f %s simple %s %s\n", diffSmpl, ckThresh(diffSmpl, simpleThresh), imageA.Path, imageB.Path)
			}
		}
		if s.CPrism == true {
			diffPrism := compareColorsPrismd(s, imageA, imageB)
			fmt.Printf("%04f %s prism  %s %s\n", diffPrism, ckThresh(diffPrism, prismThresh), imageA.Path, imageB.Path)
		}

		if s.CStdDev == true {
			diffStdDev := compareColorsStdDev(s, imageA, imageB)
			fmt.Printf("%04f %s stddev %s %s\n", diffStdDev, ckThresh(diffStdDev, stdDevThresh), imageA.Path, imageB.Path)
		}
	*/

	var pDiff float64
	switch {
	case s.Comp == 0:
		pDiff = compareColorsSimple(s, imageA, imageB)
	case s.Comp == 1:
		pDiff = compareColorsPrismd(s, imageA, imageB)
	case s.Comp == 2:
		pDiff = compareColorsStdDev(s, imageA, imageB)
	}

	matched := false
	var matchstring string
	if pDiff <= float64(s.Thresh) {
		matched = true
		matchstring = "MATCH"
	}

	if s.Verbose != true {
		if matched == false {
			return
		}
		if imageB.Size > imageA.Size {
			fmt.Printf("%s %s\n", imageB.Path, imageA.Path)
		} else {
			fmt.Printf("%s %s\n", imageA.Path, imageB.Path)
		}
		return
	}

	fmt.Printf("%04f %s %s %s\n", pDiff, matchstring, imageA.Path, imageB.Path)
	return

}

/*
func ckThresh( value float64, thresh int ) string {
	if value <= float64(thresh) {
		return "MATCH"
	}
	return ""
}
*/

func checkCompType(ctype string) int {
	/* s=simple = 0, p=prism =1, d=stddev = 2 */
	switch ctype {
	case "s":
		return 0
	case "p":
		return 1
	case "d":
		return 2
	default:
		fmt.Println("Error: compare type must be s, p, or d for simple, prism, or stddev")
		os.Exit(1)
	}
	return 0
}

func scanRecusive(dir string) ([]string, []string) {
	var images []string
	var cddata []string
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error reading dir %s: %q\n", dir, err))
		return images, cddata
	}
	for _, fi := range files {
		/*
			fi, err := os.Stat(file.Name())
			if err != nil {
				os.Stderr.WriteString(fmt.Sprintf("Error reading file %s: %q\n", file, err))
				continue
			}
		*/
		if strings.HasPrefix(fi.Name(), ".") == true {
			continue
		}
		fpath := dir + "/" + fi.Name()
		fpath = deslash(fpath)
		if fi.IsDir() == true {
			newJpegs, newCdData := scanRecusive(fpath)
			images = append(images, newJpegs...)
			cddata = append(cddata, newCdData...)
			continue
		}
		if fi.Mode().IsRegular() == true {
			switch {
			case strings.HasSuffix(fi.Name(), ".jpg"):
				images = append(images, fpath)
			case strings.HasSuffix(fi.Name(), ".png"):
				images = append(images, fpath)
			case strings.HasSuffix(fi.Name(), ".cd.gz"):
				cddata = append(cddata, fpath)
			}
		}
	}
	return images, cddata
}

func dedupe(myarray []string) []string {
	mymap := make(map[string]int)
	var newarray []string
	for _, n := range myarray {
		mymap[n] = 1
	}
	for m, _ := range mymap {
		newarray = append(newarray, m)
	}
	return newarray
}

func deslash(path string) string {
	newpath := strings.Replace(path, "//", "/", -1)
	if strings.Contains(newpath, "//") == true {
		newpath = deslash(newpath)
	}
	return newpath
}

func checkFile(path string) bool {
	_, err := os.Stat(path)
	/* could use os.IsNotExist(err) */
	if err != nil {
		return false
	}
	return true
}

func processJpeg(chData chan string, img string, s status) {

	outfile := img + ".cd.gz"
	retfile := ""
	defer func() { chData <- retfile }()

	/* if not set to overwrite, test if data file already exists */
	if s.OvrWr != true {
		if checkFile(outfile) == true {
			if s.Verbose == true {
				fmt.Printf("Skipping existing data file for %s\n", img)
			}
			return
		}
	}

	colorData, err := getColorData(img)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error getting color data from %s: %q\n", img, err))
		return
	}

	err = validateCD(colorData)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error validating generated data from %s: %q\n", img, err))
		return
	}

	j, err := json.Marshal(colorData)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error marshalling data for %s: %q\n", img, err))
		return
	}

	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write(j)
	w.Close()
	err = ioutil.WriteFile(outfile, b.Bytes(), 0644)
	if err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error writing to %s: %q\n", outfile, err))
		return
	}

	retfile = outfile
	return

}
