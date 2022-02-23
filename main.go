package main

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/alexflint/go-arg"
	"golang.org/x/image/bmp"
	"gopkg.in/vansante/go-ffprobe.v2"
)

type args struct {
	Path      string        `arg:"required,-i" help:"Path to video."`
	Interval  time.Duration `default:"0.5s" arg:"-t" help:"Time between frames."`
	Threshold int           `default:"1000" arg:"-d" help:"Threshold distance to recognize as different frame."`
	Workers   int           `default:"8" arg:"-w" help:"Number of processes to run parallelly."`
	Format    string        `default:"bmp" arg:"-f" help:"File format of output slides. Allowed values are: bmp (fastest), png (slowest) and jpg/jpeg."`
	Batch     int           `default:"0" arg:"-b" help:"Describes if Path is a path to a folder of videos."`
}

func (args) Version() string {
	return "slidextract v0.1.1"
}

func fastCompare(img1, img2 *image.RGBA) (int64, error) {
	if img1.Bounds() != img2.Bounds() {
		return 0, fmt.Errorf("image bounds not equal: %+v, %+v", img1.Bounds(), img2.Bounds())
	}
	pxcount := img1.Bounds().Dx() * img1.Bounds().Dy()
	accumError := uint64(0)

	for i := 0; i < len(img1.Pix); i++ {
		accumError += (sqDiffUInt8(img1.Pix[i], img2.Pix[i]))
	}

	fmt.Println(int64(math.Sqrt(float64(accumError)) * 100000 / float64(pxcount)))

	return int64(math.Sqrt(float64(accumError)) * 100000 / float64(pxcount)), nil
}

func getImageFromFilePath(filePath, format string) (*image.RGBA, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var img image.Image
	if format == "bmp" {
		img, err = bmp.Decode(f)
	} else if format == "png" {
		img, err = png.Decode(f)
	} else if format == "jpg" || format == "jpeg" {
		img, err = jpeg.Decode(f)
	} else {
		panic("Unrecognized format type: Allowed formats are bmp, png, and jpg/jpeg")
	}

	rect := img.Bounds()
	rgba := image.NewRGBA(rect)
	draw.Draw(rgba, rect, img, rect.Min, draw.Src)

	return rgba, err
}

func sqDiffUInt8(x, y uint8) uint64 {
	d := uint64(x) - uint64(y)
	return d * d
}

func runCommand(cmd *exec.Cmd) error {
	return cmd.Run()
}

func extractVideoFrames(path string, interval time.Duration, workers int, format string, outputDir string) (int, error) {
	var err error

	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	data, err := ffprobe.ProbeURL(ctx, path)
	if err != nil {
		log.Panicf("Error getting data: %v", err)
	}

	runningTime := data.Format.Duration()
	numFrames := runningTime / interval

	c := make(chan *exec.Cmd)

	var wg sync.WaitGroup
	wg.Add(workers)
	for ii := 0; ii < workers; ii++ {
		go func(c chan *exec.Cmd) {
			for {
				cmd, more := <-c
				if !more {
					wg.Done()
					return
				}

				err := runCommand(cmd)
				if err != nil {
					log.Println(err)
				}
			}
		}(c)
	}

	for i := 0; i < int(numFrames); i++ {

		command := "ffmpeg"
		ss := fmt.Sprint((interval * time.Duration(i)).Microseconds()) + "us"
		_, name := filepath.Split(path)
		stem := name[:len(name)-len(filepath.Ext(name))]
		cmd := exec.Command(
			command,
			"-accurate_seek",
			"-ss", ss,
			"-i", path,
			"-frames:v", "1",
			outputDir+"/frame_"+stem+"_"+fmt.Sprint(i)+"."+format,
		)

		c <- cmd
	}
	close(c)
	wg.Wait()
	return int(numFrames), err
}

func generateSlides(args args, outputDir string) {
	start := time.Now()
	OriginalStart := time.Now()
	num, err := extractVideoFrames(args.Path, args.Interval, args.Workers, args.Format, outputDir)
	if err != nil {
		panic(err)
	}
	_, name := filepath.Split(args.Path)
	stem := name[:len(name)-len(filepath.Ext(name))]
	fmt.Println("[Video : " + stem + "]")
	fmt.Println("Time for extracting frames: ", time.Since(start))

	start = time.Now()
	for i := 1; i < num; i++ {
		a, err := getImageFromFilePath(outputDir+"/frame_"+stem+"_"+fmt.Sprint(i-1)+"."+args.Format, args.Format)
		if err != nil {
			panic(err)
		}
		b, err := getImageFromFilePath(outputDir+"/frame_"+stem+"_"+fmt.Sprint(i)+"."+args.Format, args.Format)
		if err != nil {
			panic(err)
		}
		distAB, err := fastCompare(a, b)
		fmt.Println(distAB)
		if err != nil {
			panic(err)
		}
		if distAB < int64(args.Threshold) {
			os.Remove(outputDir + "/frame_" + stem + "_" + fmt.Sprint(i-1) + "." + args.Format)
		}
	}
	fmt.Println("Time for deleting similar frames: ", time.Since(start))
	fmt.Println("Total: ", time.Since(OriginalStart))
	fmt.Println()
}

func main() {
	var args args
	os.Mkdir("out", 0755)
	p := arg.MustParse(&args)

	if (args.Format != "png") && (args.Format != "bmp") && (args.Format != "jpg") && (args.Format != "jpeg") {
		p.Fail("Format can only be png, bmp, jpg/jpeg.")
	}

	if args.Batch != 0 {
		fileInfo, err := ioutil.ReadDir(args.Path)
		if err != nil {
			panic(err)
		}
		for _, file := range fileInfo {
			name := file.Name()
			stem := name[:len(name)-len(filepath.Ext(name))]
			outputDir := "out/" + stem
			err := os.Mkdir(outputDir, 0755)
			if err != nil {
				panic(err)
			}
			videoArgs := args
			if args.Path[len(args.Path)-1:] != "/" && args.Path[len(args.Path)-1:] != "\\" {
				args.Path += "/"
			}
			videoArgs.Path = args.Path + file.Name()
			generateSlides(videoArgs, outputDir)
		}
	} else {
		name := args.Path
		_, name = filepath.Split(name)
		stem := name[:len(name)-len(filepath.Ext(name))]
		outputDir := "out/" + stem
		err := os.Mkdir(outputDir, 0755)
		if err != nil {
			panic(err)
		}
		generateSlides(args, outputDir)
	}

}
