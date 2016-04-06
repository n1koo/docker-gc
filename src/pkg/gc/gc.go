package gc

import (
	"math"
	"os"
	"pkg/helpers"
	"pkg/statsd"
	"syscall"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
	"github.com/n1koo/gocron"
)

const (
	DockerEndpoint     = "unix:///var/run/docker.sock"
	StatsdSamplingRate = 1.0
	BatchSizeToDelete  = 10
	Image              = "image"
	Container          = "container"
	DatePolicy         = "date"
	DiskPolicy         = "disk"
)

var (
	Client *docker.Client
)

type DiskSpaceFetcher struct{}
type DiskSpace interface {
	GetUsedDiskSpaceInPercents() (int, error)
}

type GCPolicy struct {
	HighDiskSpaceThreshold int
	LowDiskSpaceThreshold  int
	KeepLastContainers     time.Duration
	KeepLastImages         time.Duration
}

func StartDockerClientDefault() *docker.Client {
	return StartDockerClient(DockerEndpoint)
}

func StartDockerClient(endpoint string) *docker.Client {
	var err error

	if Client != nil {
		log.Warn("Docker client already initialized, reinitialize happening")
	}

	Client, err = docker.NewClient(endpoint)
	if err != nil {
		log.WithField("error", err).Fatal("Error creating Docker client")
		os.Exit(1)
	}

	err = Client.Ping()
	if err != nil {
		log.WithField("error", err).Fatal("Error talking to Docker API when initializing client")
		os.Exit(1)
	}

	return nil
}

func DiskSpaceGC(intervalInSeconds uint64, policy GCPolicy) {
	diskSpaceFetcher := &DiskSpaceFetcher{}
	gocron.Every(intervalInSeconds).Seconds().Do(CleanAllWithDiskSpacePolicy, diskSpaceFetcher, policy)
	log.Info("Continous run started in diskspace mode with interval (in seconds): ", intervalInSeconds)
	gocron.Start()
}

func TtlGC(intervalInSeconds uint64, policy GCPolicy) {
	gocron.Every(intervalInSeconds).Seconds().Do(CleanAll, DatePolicy, policy)
	log.Info("Continous run started in timebased mode with interval (in seconds): ", intervalInSeconds)
	gocron.Start()
}

func StopGC() {
	gocron.Clear()
}

func CleanAllWithDiskSpacePolicy(diskSpaceFetcher DiskSpace, policy GCPolicy) {
	usedDiskSpace, diskErr := diskSpaceFetcher.GetUsedDiskSpaceInPercents()
	if diskErr != nil {
		log.WithField("error", diskErr).Error("Reading disk space failed")
		return
	}

	if usedDiskSpace >= policy.HighDiskSpaceThreshold {
		for usedDiskSpace > policy.LowDiskSpaceThreshold {
			log.WithFields(log.Fields{
				"currentUsedDiskSpace":   usedDiskSpace,
				"highDiskSpaceThreshold": policy.HighDiskSpaceThreshold,
				"lowDiskSpaceThreshold":  policy.LowDiskSpaceThreshold,
			}).Info("Cleaning images to reach low used disk space threshold")

			cleanedContainers, cleanedImages := CleanAll(DiskPolicy, policy)
			if cleanedContainers == 0 && cleanedImages == 0 {
				log.Info("Breaking cleanup due to no data being deleted anymore")
				break // we are not making progress, lets relax for interval period
			}

			usedDiskSpace, diskErr = diskSpaceFetcher.GetUsedDiskSpaceInPercents()
			if diskErr != nil {
				log.WithField("error", diskErr).Error("Reading disk space failed")
				break
			}
		}
	} else {
		log.WithFields(log.Fields{
			"currentUsedDiskSpace":   usedDiskSpace,
			"highDiskSpaceThreshold": policy.HighDiskSpaceThreshold,
			"lowDiskSpaceThreshold":  policy.LowDiskSpaceThreshold,
		}).Info("Disk space threshold not reached, cleaning only the containers based on TTL")
		CleanContainers(policy.KeepLastContainers)
	}
}

func CleanImages(imagesTtl time.Duration) {
	removeDataBasedOnAge(getImages(), Image, imagesTtl)
}

func CleanContainers(containersTtl time.Duration) {
	removeDataBasedOnAge(getFinishedContainers(), Container, containersTtl)
}

func CleanAll(mode string, policy GCPolicy) (int, int) {
	log.Info("Cleaning all images/containers")
	statsd.Count("clean.start", 1, []string{}, StatsdSamplingRate)

	var removedContainers int
	var removedImages int

	switch mode {
	case DiskPolicy:
		removedContainers = removeDataBasedOnAge(getFinishedContainers(), Container, policy.KeepLastContainers)
		removedImages = removeImagesInBatches(policy.KeepLastImages)
	case DatePolicy:
		removedContainers = removeDataBasedOnAge(getFinishedContainers(), Container, policy.KeepLastContainers)
		removedImages = removeDataBasedOnAge(getImages(), Image, policy.KeepLastImages)
	default:
		log.Error(mode + " is not valid policy")
		os.Exit(2)
	}
	return removedContainers, removedImages
}

func getDockerRoot() string {
	info, err := Client.Info()
	if err != nil {
		log.WithField("error", err).Error("Getting docker info failed")
		os.Exit(1)
	}

	return info.DockerRootDir
}

func getImagesInUse() []string {
	containersList := getRunningContainers()
	var usedImages []string

	for _, container := range containersList {
		usedImages = append(usedImages, container.Image)
		imageHistory, err := Client.ImageHistory(container.Image)
		if err != nil {
			log.WithField("error", err).Error("Getting image history failed")
			continue
		}
		for _, image := range imageHistory {
			usedImages = append(usedImages, image.ID)
		}
	}

	return usedImages
}

func getImages() map[int64][]string {
	imageMap := map[int64][]string{}
	imageData, err := Client.ListImages(docker.ListImagesOptions{All: true})
	if err != nil {
		log.WithField("error", err).Error("Listing images error")
		return imageMap
	}

	usedImages := getImagesInUse()

	for _, data := range imageData {
		if !helpers.StringInSlice(data.ID, usedImages) {
			imageMap[data.Created] = append(imageMap[data.Created], data.ID)
		}
	}
	statsd.Gauge("image.amount", len(imageData))
	return imageMap
}

func getFinishedContainers() map[int64][]string {
	containerMap := map[int64][]string{}

	//XXX: Support for dead is only in 1.10 https://github.com/docker/docker/pull/17908
	onlyGetExitedContainers := docker.ListContainersOptions{Filters: map[string][]string{"status": {"exited", "dead"}}}
	containersList, err := Client.ListContainers(onlyGetExitedContainers)
	if err != nil {
		log.WithField("error", err).Error("Listing containers error")
		return containerMap
	}

	for _, data := range containersList {
		containerFullData, cErr := Client.InspectContainer(data.ID)
		if cErr != nil {
			log.WithField("error", cErr).Error("Fetching container full data error")
		} else {
			date := containerFullData.State.FinishedAt.Unix()
			containerMap[date] = append(containerMap[date], data.ID)
		}

	}
	statsd.Gauge("container.dead.amount", len(containersList))
	return containerMap
}

func getRunningContainers() []docker.APIContainers {
	onlyGetExitedContainers := docker.ListContainersOptions{Filters: map[string][]string{"status": {"running"}}}
	containersList, err := Client.ListContainers(onlyGetExitedContainers)
	if err != nil {
		log.WithField("error", err).Error("Listing containers error")
		return containersList
	}
	return containersList
}

func removeImagesInBatches(keepLast time.Duration) int {
	dataMap := getImages()

	dates := helpers.SortDataMap(dataMap)
	var batch []int64
	if len(dates) > BatchSizeToDelete {
		batch = dates[len(dates)-BatchSizeToDelete:]
	} else {
		batch = dates
	}

	// Create a new map with only the values in the batch
	batchDataMap := map[int64][]string{}
	for _, date := range batch {
		batchDataMap[date] = dataMap[date]
	}

	// Respect the TTL for images to not delete all of the images in disk filling situations
	return removeDataBasedOnAge(batchDataMap, Image, keepLast)
}

func removeDataBasedOnAge(dataMap map[int64][]string, dataType string, keepLast time.Duration) int {
	var deletedData int
	dates := helpers.SortDataMap(dataMap)

	for _, date := range dates {
		for _, id := range dataMap[date] {
			ageOfData := time.Since(time.Unix(date, 0))

			// If container/image is older than our threshold, delete it
			if ageOfData > keepLast {
				log.WithFields(log.Fields{
					"type":      dataType,
					"expires":   ageOfData - keepLast,
					"age":       ageOfData,
					"threshold": keepLast,
				}).Info("Trying to delete "+dataType+": ", id)
				if succeeded := removeData(id, dataType); succeeded {
					deletedData++
				}
			}
		}
	}
	return deletedData
}

func removeData(id, dataType string) bool {
	if dataType == Image {
		err := Client.RemoveImageExtended(id, docker.RemoveImageOptions{NoPrune: true, Force: true})
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"id":    id,
			}).Error("Image deletion error")
			return false
		}
		statsd.Count("image.deleted", 1, []string{}, StatsdSamplingRate)
	} else if dataType == Container {
		err := Client.RemoveContainer(docker.RemoveContainerOptions{ID: id})
		if err != nil {
			log.WithFields(log.Fields{
				"error": err,
				"id":    id,
			}).Error("Container deletion error")
			return false
		}
		statsd.Count("container.deleted", 1, []string{}, StatsdSamplingRate)
	} else {
		log.Error("removeData called with unvalid Datatype: " + dataType)
		return false
	}
	return true
}

func (d *DiskSpaceFetcher) GetUsedDiskSpaceInPercents() (int, error) {
	path := getDockerRoot()

	s := syscall.Statfs_t{}
	err := syscall.Statfs(path, &s)

	if err != nil {
		log.WithField("error", err).Error("Getting used disk space failed")
		return 0, err
	}

	total := int(s.Bsize) * int(s.Blocks)
	free := int(s.Bsize) * int(s.Bfree)

	percent := math.Floor(100 - (float64(free) / float64(total) * 100))
	return int(percent), err
}
