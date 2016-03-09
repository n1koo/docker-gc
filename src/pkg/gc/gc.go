package gc

import (
	log "github.com/Sirupsen/logrus"
	"github.com/cznic/sortutil"
	"github.com/fsouza/go-dockerclient"
	"github.com/n1koo/gocron"
	"math"
	"os"
	"pkg/statsd"
	"sort"
	"syscall"
	"time"
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
	diskSpaceFetcher := &DiskSpaceFetcher{}
	gocron.Every(intervalInSeconds).Seconds().Do(CleanAllWithDiskSpacePolicy, diskSpaceFetcher, policy)
	log.Info("Continous run started in diskspace mode with interval (in seconds): ", intervalInSeconds)
	gocron.Start()
}

func ContinuousGC(intervalInSeconds uint64, policy GCPolicy) {
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
		log.Error("Reading disk space failed: ", diskErr)
		return
	}

	if usedDiskSpace >= policy.HighDiskSpaceThreshold {
		for usedDiskSpace > policy.LowDiskSpaceThreshold {
			log.WithFields(log.Fields{
				"currentUsedDiskSpace":   usedDiskSpace,
				"highDiskSpaceThreshold": policy.LowDiskSpaceThreshold,
			}).Info("Cleaning images to reach low used disk space threshold")
			CleanAll(DiskPolicy, policy)
			usedDiskSpace, diskErr = diskSpaceFetcher.GetUsedDiskSpaceInPercents()
			if diskErr != nil {
				log.Error("Reading disk space failed: ", diskErr)
				break
			}
		}
	} else {
		log.Info("Disk space threshold not reached, skipping cleanup")
	}
}

func CleanImages(keepLastImages time.Duration) {
	removeDataBasedOnAge(getData(Image), Image, keepLastImages)
}

func CleanContainers(keepLastContainers time.Duration) {
	removeDataBasedOnAge(getData(Container), Container, keepLastContainers)
}

func CleanAll(mode string, policy GCPolicy) {
	log.Info("Cleaning all images/containers")
	statsd.Count("clean.start", 1, []string{}, StatsdSamplingRate)

	switch mode {
	case DiskPolicy:
		removeDataBasedOnAge(getData(Container), Container, policy.KeepLastContainers)
		removeDataInBatches(getData(Image), Image, BatchSizeToDelete)
	case DatePolicy:
		removeDataBasedOnAge(getData(Container), Container, policy.KeepLastContainers)
		removeDataBasedOnAge(getData(Image), Image, policy.KeepLastImages)
	default:
		log.Error(mode + " is not valid policy")
		os.Exit(2)
	}
}

func getDockerRoot() string {
	info, err := Client.Info()
	if err != nil {
		log.Error("Getting docker info failed: ", err)
		os.Exit(1)
	}

	return info.Get("DockerRootDir")
}

func getData(dataType string) map[int64][]string {
	dataMap := map[int64][]string{}

	switch dataType {
	case Image:
		imageData, err := Client.ListImages(docker.ListImagesOptions{All: true})
		if err != nil {
			log.Error("Listing images error: ", err)
			return dataMap
		}
		for _, data := range imageData {
			dataMap[data.Created] = append(dataMap[data.Created], data.ID)
		}
		statsd.Gauge("image.amount", len(imageData))
	case Container:
		containerData, err := Client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			log.Error("Listing containers error: ", err)
			return dataMap
		}
		for _, data := range containerData {
			dataMap[data.Created] = append(dataMap[data.Created], data.ID)
		}
		statsd.Gauge("container.amount", len(containerData))
	default:
		log.Error(dataType + " is not valid type")
		os.Exit(2)
	}

	return dataMap
}

func removeDataInBatches(dataMap map[int64][]string, dataType string, batchSizeToDelete int) {
	dates := sortDataMap(dataMap)
	var batch []int64
	if len(dates) > 10 {
		batch = dates[len(dates)-10:]
	} else {
		batch = dates
	}

	for _, created := range batch {
		for _, id := range dataMap[created] {
			log.Info("Trying to delete "+dataType+": ", id)
			removeData(id, dataType)
		}
	}
}

func removeDataBasedOnAge(dataMap map[int64][]string, dataType string, keepLast time.Duration) {
	dates := sortDataMap(dataMap)

	for _, created := range dates {
		for _, id := range dataMap[created] {
			ageOfData := time.Since(time.Unix(created, 0))

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
}

func sortDataMap(dataMap map[int64][]string) []int64 {
	//Sort map based on dates to make order predictable
	var dates []int64
	for k := range dataMap {
		dates = append(dates, k)
	}
	sort.Sort(sort.Reverse(sortutil.Int64Slice(dates)))
	return dates
}

func removeData(id, dataType string) {
	if dataType == Image {
		err := Client.RemoveImageExtended(id, docker.RemoveImageOptions{Force: true})
		if err != nil {
			log.WithField("error", err).Error("Image deletion error for: ", id)
		}
		statsd.Count("image.deleted", 1, []string{}, StatsdSamplingRate)
	} else if dataType == Container {
		err := Client.RemoveContainer(docker.RemoveContainerOptions{ID: id})
		if err != nil {
			log.WithField("error", err).Error("Container deletion error for: ", id)
		}
		statsd.Count("container.deleted", 1, []string{}, StatsdSamplingRate)
	} else {
		log.Error("removeData called with unvalid Datatype: " + dataType)
	}
}

func (d *DiskSpaceFetcher) GetUsedDiskSpaceInPercents() (int, error) {
	path := getDockerRoot()
	s := syscall.Statfs_t{}
	err := syscall.Statfs(path, &s)

	if err != nil {
		log.Error("Getting used disk space failed with " + err.Error())
		return 0, err
	}

	total := int(s.Bsize) * int(s.Blocks)
	free := int(s.Bsize) * int(s.Bfree)

	percent := math.Floor(float64(free) / float64(total) * 100)
	return int(percent), err
}
