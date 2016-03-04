package gc

import (
  log "github.com/Sirupsen/logrus"
  "github.com/fsouza/go-dockerclient"
  "os"
  "time"
  "sort"
  "github.com/n1koo/gocron"
  "pkg/statsd"
  "syscall"
  "math"
)

const (
  DockerEndpoint = "unix:///var/run/docker.sock"
  StatsdSamplingRate = 1.0
)

var (
  Client *docker.Client
)

type GCPolicy struct {
  HighDiskSpaceThreshold int
  LowDiskSpaceThreshold int
  KeepLastContainers time.Duration
  KeepLastImages time.Duration
}

func StartDockerClient(endpoint ...string) *docker.Client {
  var dEndpoint string
  if len(endpoint) == 0 {
    dEndpoint = DockerEndpoint
  } else {
    dEndpoint = endpoint[0]
  }

  var err error
  Client, err = docker.NewClient(dEndpoint)
  if err != nil {
    log.Fatal(err)
    os.Exit(1)
  }

  err = Client.Ping()
  if err != nil {
    log.Fatal(err)
    os.Exit(1)
  }
  
  return nil
}

func DiskSpaceGC(intervalInSeconds uint64, policy GCPolicy) {
  gocron.Every(intervalInSeconds).Seconds().Do(MonitorDiskSpace, policy)
  log.Info("Continous run started in diskspace mode with interval (in seconds): ", intervalInSeconds)
  gocron.Start()
}

func ContinuousGC(intervalInSeconds uint64, policy GCPolicy) {
  gocron.Every(intervalInSeconds).Seconds().Do(CleanAll, policy)
  log.Info("Continous run started in timebased mode with interval (in seconds): ", intervalInSeconds)
  gocron.Start()
}

func StopGC() {
  gocron.Clear()
}

func MonitorDiskSpace(policy GCPolicy) {
  info, err := Client.Info()
  if err != nil {
    log.Error("Getting docker info failed: ", err)
    return
  }

  dockerRoot := info.Get("DockerRootDir")

  for diskSpace, diskErr := getDiskSpace(dockerRoot); diskSpace > policy.HighDiskSpaceThreshold; {
    if diskErr != nil {
    log.Error("Reading disk space failed: ", err)
      return
    }
    log.WithFields(log.Fields{
        "currentFreeDiskSpace":    diskSpace,
        "highDiskSpaceThreshold":  policy.HighDiskSpaceThreshold,
      }).Info("High disk space threshold reached, starting cleanup")
    CleanAll(policy)
  }
}

func CleanAll(policy GCPolicy) {
  log.Info("Cleaning all images/containers")
  statsd.Count("continuous.clean.start", 1, []string{}, StatsdSamplingRate)
  CleanContainers(policy.KeepLastContainers)
  CleanImages(policy.KeepLastImages)
}

func CleanContainers(keepLastContainers time.Duration) {
  conts, err := Client.ListContainers(docker.ListContainersOptions{All: true})
  if err != nil {
    log.Error("Listing containers error: ", err)
  }

  containerMap := map[string]int64{}
  for _, cont := range conts {
    containerMap[cont.ID] = cont.Created
  }
  statsd.Gauge("container.amount", len(containerMap))

  removeDataBasedOnAge(containerMap, "container", keepLastContainers)
}

func CleanImages(keepLastImages time.Duration) {
  imgs, err := Client.ListImages(docker.ListImagesOptions{All: true})
  if err != nil {
    log.Error("Listing images error: ", err)
  }

  imageMap := map[string]int64{}
  for _, img := range imgs {
    imageMap[img.ID] = img.Created
  }
  statsd.Gauge("image.amount", len(imageMap))

  removeDataBasedOnAge(imageMap, "image", keepLastImages)
}

func removeDataBasedOnAge(dataMap map[string]int64, dataType string, keepLast time.Duration) {
  //Sort map keys to make deletion order predictable
  var ids []string
  for k := range dataMap {
    ids = append(ids, k)
  }
  sort.Strings(ids)

  for _, id := range ids {
    ageOfData := time.Since(time.Unix(dataMap[id], 0))

    // If container/image is older than our threshold, delete it
    if ageOfData > keepLast {
      log.WithFields(log.Fields{
        "type":      dataType,
        "expires":   ageOfData - keepLast,
        "age":       ageOfData,
        "threshold": keepLast,
      }).Info("Trying to delete "+dataType+": ", id)
      removeData(id, dataType)
    }
  }
}

func removeData(id, dataType string) {
  if dataType == "image" {
    err := Client.RemoveImage(id)
    if err != nil {
      log.WithField("error", err).Error("Image deletion error for: ", id)
    }
    statsd.Count("image.deleted", 1, []string{}, StatsdSamplingRate)
  } else if dataType == "container" {
    err := Client.RemoveContainer(docker.RemoveContainerOptions{ID: id})
    if err != nil {
      log.WithField("error", err).Error("Container deletion error for: ", id)
    }
    statsd.Count("container.deleted", 1, []string{}, StatsdSamplingRate)
  } else {
    log.Error("removeData called with unvalid Datatype: " + dataType)
  }
}

func getDiskSpace(path string) (int, error) {
  s := syscall.Statfs_t{}
  err := syscall.Statfs(path, &s)

  if err != nil {
    log.WithField("error", err).Error("Getting free disk space failed")
    return 0, err
  }

  total := int(s.Bsize) * int(s.Blocks)
  free := int(s.Bsize) * int(s.Bfree)

  percent := math.Floor(float64(free) / float64(total) * 100)
  return int(percent), err
}
