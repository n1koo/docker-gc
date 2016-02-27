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
  KeepLastImages     time.Duration
  KeepLastContainers time.Duration
  Command            string
  IntervalForContinuousMode time.Duration
  BugsnagKey string
  StatsdAddr string
  StatsdNamespace string
)

var (
  commandFlag               = flag.String("command", "continuous", "What to clean (images|containers|all|emergency|continous")
  keepLastImagesFlag        = flag.Duration("keep_last_images", 10*time.Hour, "How old images are kept")
  keepLastContainersFlag    = flag.Duration("keep_last_containers", 1*time.Minute, "How old containers are kept")
  intervalForContinuousMode = flag.Duration("interval", 60*time.Second, "How old containers are kept")
  bugsnagKey                = flag.String("bugsnag_key", "", "Bugsnag key")
  statsdAddr                = flag.String("statsd_address", "127.0.0.1:8125", "Statsd address to emit metrics to")
  statsdNamespace           = flag.String("statsd_namespace", "borg.dockergc.", "Namespace for statsd metrics")
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
  initBugSnag(BugsnagKey)
  statsd.Configure(StatsdAddr, StatsdNamespace)

  client := gc.StartDockerClient()

  switch Command {
  case "images":
    gc.CleanImages(KeepLastImages, client)
  case "containers":
    gc.CleanContainers(KeepLastContainers, client)
  case "all":
    gc.CleanContainers(KeepLastContainers, client)
    gc.CleanImages(KeepLastImages, client)
  case "emergency":
    gc.CleanContainers(0*time.Second, client)
    gc.CleanImages(0*time.Second, client)
  case "continuous":
    interval := uint64(IntervalForContinuousMode.Seconds())
    gc.ContinuousGC(interval, KeepLastContainers, KeepLastImages, client)
    select{}
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
  StatsdAddr = *statsdAddr
  StatsdNamespace = *statsdNamespace

  if Command != "all" && Command != "images" && Command != "containers" && Command != "emergency" && Command != "continuous" {
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
