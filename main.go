package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
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
	InMemory  int           `default:"0" arg:"-m" help:"Describes if processing is to be done in memory."`
	Workers   int           `default:"8" arg:"-w" help:"Number of processes to run parallelly. NOTE: If processing is being done in memory, processing is forced single threaded."`
	MaxFrames int           `default:"25" arg:"-x" help:"[InMemory only] Maximum number of unprocessed frames to hold in memory at a time."`
	Format    string        `default:"bmp" arg:"-f" help:"File format of output slides. Allowed values are: bmp (fastest), png (slowest) and jpg/jpeg."`
	Batch     int           `default:"0" arg:"-b" help:"Describes if Path is a path to a folder of videos."`
}

func (args) Version() string {
	return "slidextract v0.2.0"
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

func runCommandInMemory(cmd *exec.Cmd) ([]byte, error) {
	pipeOut, _ := cmd.StdoutPipe()
	defer pipeOut.Close()
	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	imageBytes, err := io.ReadAll(pipeOut)
	if err != nil {
		return nil, err
	}

	return imageBytes, nil
}

func saveFileFromMemory(args args, outputDir string, globalCount int, processedFrame image.RGBA) error {
	_, name := filepath.Split(args.Path)
	stem := name[:len(name)-len(filepath.Ext(name))]
	path := outputDir + "/frame_" + stem + "_" + fmt.Sprint(globalCount) + "." + args.Format
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if args.Format == "bmp" {
		err = bmp.Encode(f, &processedFrame)
	} else if args.Format == "png" {
		err = png.Encode(f, &processedFrame)
	} else if args.Format == "jpg" || args.Format == "jpeg" {
		opt := jpeg.Options{
			Quality: 90,
		}
		err = jpeg.Encode(f, &processedFrame, &opt)
	} else {
		panic("Unrecognized format type: Allowed formats are bmp, png, and jpg/jpeg")
	}
	if err != nil {
		return err
	}
	return nil
}

func extractVideoFramesToMemory(args args, outputDir string) (int, error) {
	var err error

	ctx, cancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFn()

	data, err := ffprobe.ProbeURL(ctx, args.Path)
	if err != nil {
		log.Panicf("Error getting data from ffprobe: %v", err)
	}

	runningTime := data.Format.Duration()
	numFrames := runningTime / args.Interval

	frames := make([]image.RGBA, args.MaxFrames)
	counter := 0

	numBatches := (int(numFrames) / args.MaxFrames)
	if int(numFrames)%args.MaxFrames != 0 {
		numBatches += 1
	}
	lastBatch := int(numFrames) % args.MaxFrames

	numSaved := 0

	for i := 0; i < numBatches; i++ {

		counter = 0
		frameCount := args.MaxFrames
		if i == numBatches-1 {
			frameCount = lastBatch
		}
		for j := 0; j < frameCount; j++ {
			command := "ffmpeg"
			ss := fmt.Sprint((args.Interval * time.Duration(i*args.MaxFrames+j)).Microseconds()) + "us"
			codec := args.Format
			if args.Format == "jpeg" || args.Format == "jpg" {
				codec = "mjpeg"
			}
			cmd := exec.Command(
				command,
				"-accurate_seek",
				"-ss", ss,
				"-i", args.Path,
				"-frames:v", "1",
				"-c:v", codec,
				"-f", "image2pipe",
				"pipe:1",
			)

			imageBytes, err := runCommandInMemory(cmd)
			if err != nil {
				return 0, err
			}
			buf := bytes.NewBuffer(imageBytes)

			var picDat image.Image
			if args.Format == "bmp" {
				picDat, err = bmp.Decode(buf)
			} else if args.Format == "png" {
				picDat, err = png.Decode(buf)
			} else if args.Format == "jpg" || args.Format == "jpeg" {
				picDat, err = jpeg.Decode(buf)
			} else {
				panic("Unrecognized format type: Allowed formats are bmp, png, and jpg/jpeg")
			}

			if err != nil {
				return 0, err
			}
			rect := picDat.Bounds()
			rgba := image.NewRGBA(rect)
			draw.Draw(rgba, rect, picDat, rect.Min, draw.Src)
			frames[counter] = *rgba
			counter++
		}

		cursor := 0
		numSaved++
		saveFileFromMemory(args, outputDir, numSaved, frames[cursor])

		for k := 1; k < frameCount; k++ {
			distAB, err := fastCompare(&frames[cursor], &frames[k])
			if err != nil {
				panic(err)
			}
			if distAB >= int64(args.Threshold) {
				cursor = k
				numSaved++
				saveFileFromMemory(args, outputDir, numSaved, frames[k])
			}
		}
	}

	return int(numFrames), err
}

func generateSlidesInMemory(args args, outputDir string) {
	OriginalStart := time.Now()
	_, err := extractVideoFramesToMemory(args, outputDir)
	if err != nil {
		panic(err)
	}
	fmt.Println("Total: ", time.Since(OriginalStart))
	fmt.Println()
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
			if args.InMemory != 0 {
				generateSlidesInMemory(videoArgs, outputDir)
			} else {
				generateSlides(videoArgs, outputDir)
			}
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
		if args.InMemory != 0 {
			generateSlidesInMemory(args, outputDir)
		} else {
			generateSlides(args, outputDir)
		}
	}
}
