package main

import (
	"os"
	"sort"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/fsouza/go-dockerclient"
	"github.com/n1koo/gocron"
)

const dockerEndpoint = "unix:///var/run/docker.sock"

func startDockerClient(endpoint ...string) *docker.Client {
	var dEndpoint string
	if len(endpoint) == 0 {
		dEndpoint = dockerEndpoint
	} else {
		dEndpoint = endpoint[0]
	}

	client, err := docker.NewClient(dEndpoint)
	if err != nil {
		log.Fatal(err)
		os.Exit(1)
	}
	return client
}

func continuousGC(intervalInSeconds uint64, keepLastContainers time.Duration, keepLastImages time.Duration, client *docker.Client) {
	gocron.Every(intervalInSeconds).Seconds().Do(cleanAll, keepLastContainers, keepLastImages, client)
	log.Info("Continous run started with interval (in seconds): ", intervalInSeconds)
	gocron.Start()
}

func stopGC() {
	gocron.Clear()
}

func cleanAll(keepLastContainers time.Duration, keepLastImages time.Duration, client *docker.Client) {
	log.Info("Cleaning all images/containers")
	cleanContainers(keepLastContainers, client)
	cleanImages(keepLastImages, client)
}

func cleanContainers(keepLast time.Duration, client *docker.Client) {
	conts, err := client.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		log.Error("Listing containers error: ", err)
	}

	containerMap := map[string]int64{}
	for _, cont := range conts {
		containerMap[cont.ID] = cont.Created
	}

	removeData(containerMap, "container", keepLast, client)
}

func cleanImages(keepLast time.Duration, client *docker.Client) {
	imgs, err := client.ListImages(docker.ListImagesOptions{All: true})
	if err != nil {
		log.Error("Listing images error: ", err)
	}

	imageMap := map[string]int64{}
	for _, img := range imgs {
		imageMap[img.ID] = img.Created
	}

	removeData(imageMap, "image", keepLast, client)
}

func removeData(dataMap map[string]int64, dataType string, keepLast time.Duration, client *docker.Client) {
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
			} else if dataType == "container" {
				opts := docker.RemoveContainerOptions{ID: id}
				err := client.RemoveContainer(opts)
				if err != nil {
					log.WithField("error", err).Error("Container deletion error for: ", id)
				}
			} else {
				log.Error("removeData called with unvalid Datatype: " + dataType)
			}
		}
	}
}
