package main

import "fmt"
import "github.com/gographics/imagick/imagick"

/*
 *  https://gowalker.org/github.com/gographics/imagick/imagick
 */

func main() {

	imagick.Initialize()
	defer imagick.Terminate()

	mw := imagick.NewMagickWand()
	defer mw.Destroy()

	err := mw.ReadImage("IMG_0696.jpg")
	if err != nil {
		fmt.Println("Error: ", err)
	}

	length, err := mw.GetImageLength()

	fmt.Println("Hello, world: ", length)

}
