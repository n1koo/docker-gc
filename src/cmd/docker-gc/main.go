package main

import (
	"flag"
	"fmt"
	"os"
	"pkg/gc"
	"pkg/statsd"
	"time"

	logrus_bugsnag "github.com/Shopify/logrus-bugsnag"
	log "github.com/Sirupsen/logrus"
	"github.com/bugsnag/bugsnag-go"
)

var (
	command                   string
	intervalForContinuousMode time.Duration
	bugsnagKey                string
	statsdAddr                string
	statsdNamespace           string
	gcPolicy                  gc.GCPolicy
)

var (
	commandFlag                   = flag.String("command", "ttl", "What to clean (images|containers|all|emergency|continous")
	imagesTtlFlag                 = flag.Duration("images_ttl", 10*time.Hour, "How old images are kept")
	containersTtlFlag             = flag.Duration("containers_ttl", 1*time.Minute, "How old containers are kept")
	intervalForContinuousModeFlag = flag.Duration("interval", 60*time.Second, "How often we run checks in interval mode")
	bugsnagKeyFlag                = flag.String("bugsnag_key", "", "Bugsnag key")
	statsdAddrFlag                = flag.String("statsd_address", "127.0.0.1:8125", "Statsd address to emit metrics to")
	statsdNamespaceFlag           = flag.String("statsd_namespace", "borg.dockergc.", "Namespace for statsd metrics")
	highDiskSpaceThresholdFlag    = flag.Int("high_disk_space_threshold", 85, "High disk space threshold for GC in percentage")
	lowDiskSpaceThresholdFlag     = flag.Int("low_disk_space_threshold", 50, "Low disk space threshold for GC in percentage")
)

const usageMessage = `Usage of 'docker-gc':
  docker-gc -command=containers|images|all|emergency [-images_ttl=<DURATION>) [-containers_ttl=<DURATION>]
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc -command=ttl [-interval=<INTERVAL_IN_SECONDS>] [-images_ttl=<DURATION>] [-containers_ttl=<DURATION>] for continuous cleanup based on image/container TTL
  OR
  docker-gc -command=diskspace [-interval=<INTERVAL_IN_SECONDS>] [-high_disk_space_threshold=<PERCENTAGE>] [-low_disk_space_threshold=<PERCENTAGE>] [-containers_ttl=<DURATION>] for continuous cleanup based on used disk space

  You can also specify -bugsnag-key="key" to use bugsnag integration
  and [-statsd_address=<127.0.0.1:815>] and [statsd_namespace=<docker.gc.wtf>] for statsd integration
`

func main() {
	parseFlags()
	initBugSnag(bugsnagKey)
	statsd.Configure(statsdAddr, statsdNamespace)
	gc.StartDockerClientDefault()

	switch command {
	case "images":
		gc.CleanImages(gcPolicy.TtlImages)
	case "containers":
		gc.CleanContainers(gcPolicy.TtlContainers)
	case "all":
		gc.CleanAll(gc.DatePolicy, gcPolicy)
	case "emergency":
		emergencyPolicy := gc.GCPolicy{TtlContainers: 0, TtlImages: 0}
		gc.CleanAll(gc.DatePolicy, emergencyPolicy)
	case "ttl":
		interval := uint64(intervalForContinuousMode.Seconds())
		gc.TtlGC(interval, gcPolicy)
		select {}
	case "diskspace":
		interval := uint64(intervalForContinuousMode.Seconds())
		gc.DiskSpaceGC(interval, gcPolicy)
		select {}
	default:
		log.Error(command + " is not valid command")
		Usage()
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

	command = *commandFlag
	intervalForContinuousMode = *intervalForContinuousModeFlag
	statsdAddr = *statsdAddrFlag
	statsdNamespace = *statsdNamespaceFlag

	gcPolicy.TtlImages = *imagesTtlFlag
	gcPolicy.TtlContainers = *containersTtlFlag
	gcPolicy.HighDiskSpaceThreshold = *highDiskSpaceThresholdFlag
	gcPolicy.LowDiskSpaceThreshold = *lowDiskSpaceThresholdFlag

	if gcPolicy.HighDiskSpaceThreshold > 100 || gcPolicy.HighDiskSpaceThreshold < 0 ||
		gcPolicy.LowDiskSpaceThreshold > gcPolicy.HighDiskSpaceThreshold || gcPolicy.LowDiskSpaceThreshold < 0 {
		log.Error("Disk space threshold not valid, check that values are valid percentage values between 0-100 and that high is bigger than low")
		flag.Usage()
		os.Exit(2)
	}
}

func initBugSnag(bugsnagKey string) {
	if bugsnagKey != "" {
		bugsnag.Configure(bugsnag.Configuration{
			APIKey: bugsnagKey,
		})

		hook, err := logrus_bugsnag.NewBugsnagHook()
		if err != nil {
			log.WithField("error", err).Error("Failed to initialize bugsnag hook")
		} else {
			log.AddHook(hook)
		}
	}
}
