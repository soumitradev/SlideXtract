# SlideXtract

A tool to help extract slides from a video file.

Slides are output in the `out` folder.

## Features

I didn't find any other piece of code that did all the stuff I wanted, so I'll talk about some of the unique things about my code:

- Customizable interval between frames, so you don't end up extracting every single frame in the video.
- Customizable threshold distance to help differentiate between slides. Can be adjusted to fit your video better.
- Parallel processing while extracting frames from video
- Batch processing
- In-memory processing

## Requirements

- `ffmpeg` and `ffprobe` on `PATH`.

## Usage

```
Usage: slidextract --path PATH [--interval INTERVAL] [--threshold THRESHOLD] [--inmemory INMEMORY] [--workers WORKERS] [--maxframes MAXFRAMES] [--format FORMAT] [--batch BATCH]

Options:
  --path PATH, -i PATH   Path to video.
  --interval INTERVAL, -t INTERVAL
                         Time between frames. [default: 0.5s]
  --threshold THRESHOLD, -d THRESHOLD
                         Threshold distance to recognize as different frame. [default: 1000]
  --inmemory INMEMORY, -m INMEMORY
                         Describes if processing is to be done in memory. [default: 0]
  --workers WORKERS, -w WORKERS
                         Number of processes to run parallelly. NOTE: If processing is being done in memory, processing is forced single threaded. [default: 8]
  --maxframes MAXFRAMES, -x MAXFRAMES
                         [InMemory only] Maximum number of unprocessed frames to hold in memory at a time. [default: 25]
  --format FORMAT, -f FORMAT
                         File format of output slides. Allowed values are: bmp (fastest), png (slowest) and jpg/jpeg. [default: bmp]
  --batch BATCH, -b BATCH
                         Describes if Path is a path to a folder of videos. [default: 0]
  --help, -h             display this help and exit
  --version              display version and exit
```

For some reason the `bmp` format is fastest, and the `png` format is slowest. This probably has something to do with codecs, and how `ffmpeg` handles stuff. This has been tested on a real lecture (about an hour long) with a 5 second interval between frames.

Slides are output in the `out` folder.

## Why?

Every single day I lose another bit of my sanity. I am on the brink of losing it.

Turns out, some professors will use a presentation in class, but will _refuse_. I REPEAT, **REFUSE** TO GIVE YOU THE SLIDES. THEY'LL UPLOAD A RECORDING OF THE LECTURE BUT WILL REFUUUUUUUSE TO UPLOAD THE SLIDES.

So, I made this. Have fun :)

<sup>Unis will do anything in their power to cause you maximum pain. I hope this relieved some of it.</sup>

## Contribution

So far, some things we can improve on:

- Speeding up frame extraction if possible
- Speeding up the algorithm by which we find distance between frames
- Alternate image distance algorithms
- Hardware acceleration support for `ffmpeg`

If you have more ideas, feel free to open a PR/Issue!
