package main

import (
  "flag"
  "fmt"
  log "github.com/Sirupsen/logrus"
  "os"
  "time"
  "github.com/Shopify/logrus-bugsnag"
  "github.com/bugsnag/bugsnag-go"
  "pkg/statsd"
  "pkg/gc"
)

var (
  command            string
  intervalForContinuousMode time.Duration
  bugsnagKey string
  statsdAddr string
  statsdNamespace string
  imageGCPolicy gc.GCPolicy
)

var (
  commandFlag                   = flag.String("command", "continuous", "What to clean (images|containers|all|emergency|continous")
  keepLastImagesFlag            = flag.Duration("keep_last_images", 10*time.Hour, "How old images are kept")
  keepLastContainersFlag        = flag.Duration("keep_last_containers", 1*time.Minute, "How old containers are kept")
  intervalForContinuousModeFlag = flag.Duration("interval", 60*time.Second, "How old containers are kept")
  bugsnagKeyFlag                = flag.String("bugsnag_key", "", "Bugsnag key")
  statsdAddrFlag                = flag.String("statsd_address", "127.0.0.1:8125", "Statsd address to emit metrics to")
  statsdNamespaceFlag           = flag.String("statsd_namespace", "borg.dockergc.", "Namespace for statsd metrics")
  highDiskSpaceThresholdFlag    = flag.Int("high_disk_space_threshold", 85, "High disk space threshold for GC in percentage")
  lowDiskSpaceThresholdFlag     = flag.Int("low_disk_space_threshold", 50, "High disk space threshold for GC in percentage")
)

const usageMessage = `Usage of 'docker-gc':
  docker-gc (-command=containers|images|all|emergency) (-keep_last_images=DURATION) (-keep_last_containers=DURATION)
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc (-command=continuous) (-interval=INTERVAL_IN_SECONDS) (-keep_last_images=DURATION) (-keep_last_containers=DURATION) for continuous cleanup 

  You can also specify -bugsnag-key="key" to use bugsnag integration
  and -statsd_address=127.0.0.1:815 and statsd_namespace=docker.gc.wtf. for statsd integration
`

func main() {
  parseFlags()
  initBugSnag(bugsnagKey)
  statsd.Configure(statsdAddr, statsdNamespace)
  gc.StartDockerClient()

  switch command {
  case "images":
    gc.CleanImages(imageGCPolicy.KeepLastImages)
  case "containers":
    gc.CleanContainers(imageGCPolicy.KeepLastContainers)
  case "all":
    gc.CleanAll(imageGCPolicy)
  case "emergency":
    emergencyPolicy := gc.GCPolicy { KeepLastContainers: 0, KeepLastImages: 0 }
    gc.CleanAll(emergencyPolicy)
  case "continuous":
    interval := uint64(intervalForContinuousMode.Seconds())
    gc.ContinuousGC(interval, imageGCPolicy)
    select{}
  case "diskspace":
    interval := uint64(intervalForContinuousMode.Seconds())
    gc.DiskSpaceGC(interval, imageGCPolicy)
    select{}
  default:
    log.Error("%q is not valid command.\n", command)
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

  imageGCPolicy.KeepLastImages = *keepLastImagesFlag
  imageGCPolicy.KeepLastContainers = *keepLastContainersFlag
  imageGCPolicy.HighDiskSpaceThreshold = *highDiskSpaceThresholdFlag
  imageGCPolicy.LowDiskSpaceThreshold = *lowDiskSpaceThresholdFlag

  if imageGCPolicy.HighDiskSpaceThreshold > 100 || imageGCPolicy.HighDiskSpaceThreshold < 0 || 
     imageGCPolicy.LowDiskSpaceThreshold > imageGCPolicy.HighDiskSpaceThreshold || imageGCPolicy.LowDiskSpaceThreshold < 0 {
    log.Error("Disk space threshold not valid, check that values are valid percentage values between 0-100 and that high is bigger than low")
    flag.Usage()
    os.Exit(2)
  }

  if command != "all" && command != "images" && command != "containers" && 
     command != "emergency" && command != "continuous" && command != "diskspace" {
    log.WithField("command", command).Error("Given command was not recognized")
    flag.Usage()
    os.Exit(2)
  }
}

func initBugSnag(bugsnagKey string) {
  if bugsnagKey != ""  {
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
