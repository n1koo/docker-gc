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
	Client           *docker.Client
	diskSpaceFetcher DiskSpace
)

type DiskSpaceFetcher struct{}
type DiskSpace interface {
	GetUsedDiskSpaceInPercents() (int, error)
}

type GCPolicy struct {
	HighDiskSpaceThreshold int
	LowDiskSpaceThreshold  int
	TtlContainers          time.Duration
	TtlImages              time.Duration
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
	diskSpaceFetcher = &DiskSpaceFetcher{}
	gocron.Every(intervalInSeconds).Seconds().Do(CleanAllWithDiskSpacePolicy, policy)
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

func CleanAllWithDiskSpacePolicy(policy GCPolicy) {
	usedDiskSpace, diskErr := diskSpaceFetcher.GetUsedDiskSpaceInPercents()
	if diskErr != nil {
		log.WithField("error", diskErr).Error("Reading disk space failed")
		return
	}

	if usedDiskSpace >= policy.HighDiskSpaceThreshold {
		log.WithFields(log.Fields{
			"currentUsedDiskSpace":   usedDiskSpace,
			"highDiskSpaceThreshold": policy.HighDiskSpaceThreshold,
			"lowDiskSpaceThreshold":  policy.LowDiskSpaceThreshold,
		}).Info("Cleaning images to reach low used disk space threshold")
		cleanedContainers, cleanedImages := CleanAll(DiskPolicy, policy)
		usedDiskSpace, diskErr := diskSpaceFetcher.GetUsedDiskSpaceInPercents()
		if diskErr != nil {
			log.WithField("error", diskErr).Error("Reading disk space failed")
			return
		}
		log.WithFields(log.Fields{
			"cleanedContainer": cleanedContainers,
			"cleanedImages":    cleanedImages,
			"usedDiskSpace":    usedDiskSpace,
		}).Info("Cleaning images finished")
	} else {
		log.WithFields(log.Fields{
			"currentUsedDiskSpace":   usedDiskSpace,
			"highDiskSpaceThreshold": policy.HighDiskSpaceThreshold,
			"lowDiskSpaceThreshold":  policy.LowDiskSpaceThreshold,
		}).Info("Disk space threshold not reached, cleaning only the containers based on TTL")
		CleanContainers(policy.TtlContainers)
	}
}

func CleanImages(ttl time.Duration) int {
	return removeDataBasedOnAge(getImages(), Image, ttl)
}

func CleanContainers(ttl time.Duration) int {
	return removeDataBasedOnAge(getFinishedContainers(), Container, ttl)
}

func CleanAll(mode string, policy GCPolicy) (int, int) {
	log.Info("Cleaning all images/containers")
	statsd.Count("clean.start", 1, []string{}, StatsdSamplingRate)

	var removedContainers int
	var removedImages int

	switch mode {
	case DiskPolicy:
		removedContainers = removeDataBasedOnAge(getFinishedContainers(), Container, policy.TtlContainers)
		removedImages = removeImagesInBatch(policy)
	case DatePolicy:
		removedContainers = removeDataBasedOnAge(getFinishedContainers(), Container, policy.TtlContainers)
		removedImages = removeDataBasedOnAge(getImages(), Image, policy.TtlImages)
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
	options := docker.ListContainersOptions{Filters: map[string][]string{"status": {"exited", "dead"}}}
	exited, err := Client.ListContainers(options)
	if err != nil {
		log.WithField("error", err).Error("Listing containers error")
		return containerMap
	}

	for _, data := range exited {
		data, cErr := Client.InspectContainer(data.ID)
		if cErr != nil {
			log.WithField("error", cErr).Error("Fetching container full data error")
		} else {
			date := data.State.FinishedAt.Unix()
			containerMap[date] = append(containerMap[date], data.ID)
		}

	}
	statsd.Gauge("container.dead.amount", len(exited))
	return containerMap
}

func getRunningContainers() []docker.APIContainers {
	options := docker.ListContainersOptions{Filters: map[string][]string{"status": {"running"}}}
	running, err := Client.ListContainers(options)
	if err != nil {
		log.WithField("error", err).Error("Listing containers error")
	}
	return running
}

func removeImagesInBatch(policy GCPolicy) int {
	dataMap := getImages()

	totalDeletedImages := 0
	batchCounter := 0

	dates := helpers.SortDataMap(dataMap)

	usedDiskSpace, diskErr := diskSpaceFetcher.GetUsedDiskSpaceInPercents()
	if diskErr != nil {
		log.WithField("error", diskErr).Error("Reading disk space failed")
		return 0
	}

	for usedDiskSpace > policy.LowDiskSpaceThreshold {
		var batch []int64
		if len(dates) > BatchSizeToDelete {
			// Two pointers to move so that we can have like 0:10, 10:20 etc
			start := BatchSizeToDelete * batchCounter
			end := start + BatchSizeToDelete

			if start >= len(dates) {
				break
			}

			if end > len(dates) {
				end = len(dates)
			}

			batch = dates[start:end]
		} else {
			batch = dates
		}

		// Create a new map with only the values in the batch
		batchDataMap := map[int64][]string{}
		for _, date := range batch {
			//Notice this might not be exactly BatchSizeToDelete because there might multiple images created at same exact moment
			batchDataMap[date] = dataMap[date]
		}

		totalDeletedImages = totalDeletedImages + removeDataBasedOnAge(batchDataMap, Image, policy.TtlImages)
		batchCounter++

		usedDiskSpace, diskErr = diskSpaceFetcher.GetUsedDiskSpaceInPercents()
		if diskErr != nil {
			log.WithField("error", diskErr).Error("Reading disk space failed")
			break
		}
	}
	// Respect the TTL for images to not delete all of the images in disk filling situations
	return totalDeletedImages
}

func removeDataBasedOnAge(dataMap map[int64][]string, dataType string, keepLast time.Duration) int {
	var deletedData int
	dates := helpers.SortDataMapReverse(dataMap)
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
		// Prune false : don't delete untagged parents automatically since those might still be inside accepted TTL
		// Force true : delete tagged images (since we dont want to explicitely call out to untag first)
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
