package gc

import (
  log "github.com/Sirupsen/logrus"
  "github.com/fsouza/go-dockerclient"
  "os"
  "time"
  "sort"
  "github.com/n1koo/gocron"
  "pkg/statsd"
)

const (
  DockerEndpoint = "unix:///var/run/docker.sock"
  StatsdSamplingRate = 0.1
)

func StartDockerClient(endpoint ...string) *docker.Client {
  var dEndpoint string
  if len(endpoint) == 0 {
    dEndpoint = DockerEndpoint
  } else {
    dEndpoint = endpoint[0]
  }

  client, err := docker.NewClient(dEndpoint)
  if err != nil {
    log.Fatal(err)
    os.Exit(1)
  }

  err = client.Ping()
  if err != nil {
    log.Fatal(err)
    os.Exit(1)
  }
  
  return client
}

func ContinuousGC(intervalInSeconds uint64, keepLastContainers time.Duration, keepLastImages time.Duration, client *docker.Client) {
  gocron.Every(intervalInSeconds).Seconds().Do(CleanAll, keepLastContainers, keepLastImages, client)
  log.Info("Continous run started with interval (in seconds): ", intervalInSeconds)
  gocron.Start()
}

func StopGC() {
  gocron.Clear()
}

func CleanAll(keepLastContainers time.Duration, keepLastImages time.Duration, client *docker.Client) {
  log.Info("Cleaning all images/containers")
  CleanContainers(keepLastContainers, client)
  CleanImages(keepLastImages, client)
}

func CleanContainers(keepLast time.Duration, client *docker.Client) {
  conts, err := client.ListContainers(docker.ListContainersOptions{All: true})
  if err != nil {
    log.Error("Listing containers error: ", err)
  }

  containerMap := map[string]int64{}
  for _, cont := range conts {
    containerMap[cont.ID] = cont.Created
  }

  RemoveData(containerMap, "container", keepLast, client)
}

func CleanImages(keepLast time.Duration, client *docker.Client) {
  imgs, err := client.ListImages(docker.ListImagesOptions{All: true})
  if err != nil {
    log.Error("Listing images error: ", err)
  }

  imageMap := map[string]int64{}
  for _, img := range imgs {
    imageMap[img.ID] = img.Created
  }

  RemoveData(imageMap, "image", keepLast, client)
}

func RemoveData(dataMap map[string]int64, dataType string, keepLast time.Duration, client *docker.Client) {
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

      if dataType == "image" {
        err := client.RemoveImage(id)
        if err != nil {
          log.WithField("error", err).Error("Image deletion error for: ", id)
        }
        statsd.Count("image.deleted", 1, []string{}, StatsdSamplingRate)
      } else if dataType == "container" {
        err := client.RemoveContainer(docker.RemoveContainerOptions{ID: id})
        if err != nil {
          log.WithField("error", err).Error("Container deletion error for: ", id)
        }
        statsd.Count("container.deleted", 1, []string{}, StatsdSamplingRate)
      } else {
        log.Error("removeData called with unvalid Datatype: " + dataType)
      }
    }
  }
}
