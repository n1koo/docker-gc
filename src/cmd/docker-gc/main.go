package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	log "github.com/Sirupsen/logrus"
)

var (
	KeepLastImages            time.Duration
	KeepLastContainers        time.Duration
	Command                   string
	IntervalForContinuousMode time.Duration
)

var (
	commandFlag               = flag.String("command", "continuous", "What to clean (images|containers|all|emergency|continous")
	keepLastImagesFlag        = flag.Duration("keep_last_images", 10*time.Hour, "How old images are kept")
	keepLastContainersFlag    = flag.Duration("keep_last_containers", 1*time.Minute, "How old containers are kept")
	intervalForContinuousMode = flag.Duration("interval", 60*time.Second, "How old containers are kept")
)

const usageMessage = `Usage of 'docker-gc':
  docker-gc (-command=containers|images|all|emergency) (-keep_last_images=DURATION) (-keep_last_containers=DURATION)
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc (-command=continuous) (-interval=INTERVAL_IN_SECONDS) (-keep_last_images=DURATION) (-keep_last_containers=DURATION) for continuous cleanup
`

func main() {
	parseFlags()
	client := startDockerClient()

	switch Command {
	case "images":
		cleanImages(KeepLastImages, client)
	case "containers":
		cleanContainers(KeepLastContainers, client)
	case "all":
		cleanContainers(KeepLastContainers, client)
		cleanImages(KeepLastImages, client)
	case "emergency":
		cleanContainers(0*time.Second, client)
		cleanImages(0*time.Second, client)
	case "continuous":
		interval := uint64(IntervalForContinuousMode.Seconds())
		continuousGC(interval, KeepLastContainers, KeepLastImages, client)
		select {}
	default:
		log.Error("%q is not valid command.\n", Command)
		os.Exit(2)
	}
}

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintln(os.Stderr, usageMessage)
	fmt.Fprintln(os.Stderr, "Flags:")
	flag.PrintDefaults()
	os.Exit(2)
}

func parseFlags() {
	flag.Usage = Usage
	flag.Parse()

	Command = *commandFlag
	KeepLastImages = *keepLastImagesFlag
	KeepLastContainers = *keepLastContainersFlag
	IntervalForContinuousMode = *intervalForContinuousMode

	if Command != "all" && Command != "images" && Command != "containers" && Command != "emergency" && Command != "continuous" {
		flag.Usage()
		os.Exit(2)
	}
}
