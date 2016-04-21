package gc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"pkg/statsd"
	"strings"
	"testing"
	"time"

	log "github.com/Sirupsen/logrus"
	logrustest "github.com/Sirupsen/logrus/hooks/test"
	udp "github.com/n1koo/go-udp-testing"
	"github.com/stretchr/testify/assert"
)

type FakeDiskSpaceFetcher struct {
	counter int
}

func (d *FakeDiskSpaceFetcher) GetUsedDiskSpaceInPercents() (int, error) {
	if d.counter == 0 {
		d.counter = 10
	}
	d.counter--
	return d.counter, nil
}

type testResponseMap map[string][]response

type response struct {
	method   string
	params   string
	response string
}

type containerListInfo struct {
	Id    string `json:"Id"`
	Image string `json:"Image"`
}

type state struct {
	Running    bool   `json:"Running"`
	FinishedAt string `json:"FinishedAt"`
}

type containerFullInfo struct {
	Id    string `json:"Id"`
	State state  `json:"State"`
}

type idAndCreated struct {
	Id      string `json:"Id"`
	Created int64  `json:"Created"`
}

type filters struct {
	Status []string `json:"status"`
}

func testServer(routes testResponseMap, hitsPerPath *map[string]int) *httptest.Server {
	mux := http.NewServeMux()

	for path, responses := range routes {
		// Variable shadowing.
		_responses := responses
		_path := path

		fun := func(w http.ResponseWriter, r *http.Request) {
			if val, ok := (*hitsPerPath)[_path]; ok {
				(*hitsPerPath)[_path] = val + 1
			} else {
				(*hitsPerPath)[_path] = 1
			}

			w.Header().Set("Content-Type", "application/json")

			reqParameters := r.URL.Query()
			// look for an exact method and parameter match
			for _, response := range _responses {
				if response.method == r.Method {
					parameterAndValue := strings.SplitN(response.params, "=", 2)
					// first index is the param and second value eg. ..yourthing?foo=bar, 0 = foo, 1 = bar
					foundParameterValue := strings.Join(reqParameters[parameterAndValue[0]], ",")
					//Check that we have a value for the param and that the found param is the same we have specified
					if len(parameterAndValue) > 1 && foundParameterValue == parameterAndValue[1] {
						w.WriteHeader(http.StatusOK)
						w.Write([]byte(response.response))
						return
					}
				}
			}

			// no exact match, look for method and default parameter match
			for _, response := range _responses {
				if (response.method == r.Method) && (response.params == "default") {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response.response))
					return
				}
			}

			w.WriteHeader(http.StatusNotFound)
		}
		mux.HandleFunc(path, fun)
	}
	server := httptest.NewServer(mux)
	return server
}

func generateFiveTestImages() (string, map[string]string) {
	//Generate 4 images which 2 have been exited in past minute and two havent
	timeNow := time.Now()
	threeSecondsOld := timeNow.Add(-3 * time.Second)
	fiveMinutesOld := timeNow.Add(-5 * time.Minute)
	twelweHoursOld := timeNow.Add(-12 * time.Hour)
	dayOld := timeNow.Add(-24 * time.Hour)

	idsAndDatesMap := make(map[string]int64)
	idsAndDatesMap["8dfafdbc3a40"] = timeNow.Unix()
	idsAndDatesMap["9cd87474be90"] = threeSecondsOld.Unix()
	idsAndDatesMap["3176a2479c92"] = fiveMinutesOld.Unix()
	idsAndDatesMap["4cb07b47f9fb"] = twelweHoursOld.Unix()
	idsAndDatesMap["5c76a2479c92"] = dayOld.Unix()

	var imageList []idAndCreated
	imageHistoryList := make(map[string]string)

	for id, date := range idsAndDatesMap {
		imageListInfo := idAndCreated{Id: id, Created: date}
		imageList = append(imageList, imageListInfo)

		imageHistory := mustMarshal(idAndCreated{Id: id, Created: date})
		imageHistoryList[id] = string(imageHistory)
	}

	imageListAsJson := mustMarshal(imageList)

	return string(imageListAsJson), imageHistoryList
}

func mustMarshal(data interface{}) []byte {
	out, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}

	return out
}

func generateFiveTestContainers() (string, map[string]string) {
	//Generate 4 containers of which 2 have been exited in past minute and two havent
	timeNow := time.Now()
	fiveMinutesOld := timeNow.Add(-3 * time.Second)
	twelweHoursOld := timeNow.Add(-12 * time.Hour)
	weekOld := timeNow.Add(-7 * 24 * time.Hour)
	twoWeekOld := timeNow.Add(2 * -7 * 24 * time.Hour)

	idsAndDatesMap := make(map[string]string)
	idsAndDatesMap["8dfafdbc3a40"] = timeNow.Format(time.RFC3339)
	idsAndDatesMap["9cd87474be90"] = fiveMinutesOld.Format(time.RFC3339)
	idsAndDatesMap["3176a2479c92"] = twelweHoursOld.Format(time.RFC3339)
	idsAndDatesMap["4cb07b47f9fb"] = weekOld.Format(time.RFC3339)
	idsAndDatesMap["5c76a2479c92"] = twoWeekOld.Format(time.RFC3339)

	var containerList []containerListInfo
	containerListWithFullData := make(map[string]string)

	for id, date := range idsAndDatesMap {
		containerListInfo := containerListInfo{Id: id, Image: id}
		containerList = append(containerList, containerListInfo)

		containerFullInfoJson := mustMarshal(containerFullInfo{Id: id, State: state{Running: false, FinishedAt: date}})
		containerListWithFullData[id] = string(containerFullInfoJson)
	}

	containerListAsJson := mustMarshal(&containerList)

	return string(containerListAsJson), containerListWithFullData
}

func generateTestData() testResponseMap {
	imageListAsJson, imageHistoryList := generateFiveTestImages()
	containerListAsJson, containerListWithFullData := generateFiveTestContainers()

	responses := make(testResponseMap)

	responses["/_ping"] = []response{
		{"GET", "default", "OK"}}

	responses["/images/json"] = []response{
		{"GET", "all=1", imageListAsJson}}

	runningFilter := mustMarshal(filters{Status: []string{"running"}})
	exitedFilter := mustMarshal(filters{Status: []string{"exited", "dead"}})

	responses["/containers/json"] = []response{
		{"GET", "default", containerListAsJson},
		{"GET", fmt.Sprintf("filters=%s", string(runningFilter)), "[]"},
		{"GET", fmt.Sprintf("filters=%s", string(exitedFilter)), containerListAsJson}}

	for id, data := range containerListWithFullData {
		responses["/containers/"+id+"/json"] = []response{
			{"GET", "default", data}}
		responses["/containers/"+id+""] = []response{
			{"DELETE", "default", "OK"}}
	}

	for id, data := range imageHistoryList {
		responses["/images/"+id+""] = []response{
			{"DELETE", "default", "OK"}}
		responses["/images/"+id+"/history"] = []response{
			{"GET", "default", data}}
	}

	return responses
}

func TestStartDockerClient(t *testing.T) {
	responses := make(testResponseMap)

	responses["/_ping"] = []response{
		{"GET", "default", "OK"}}

	hitsPerPath := map[string]int{}
	server := testServer(responses, &hitsPerPath)
	defer server.Close()

	endpoint := server.URL
	StartDockerClient(endpoint)
	assert.NotNil(t, Client, "Docker client should not be nil after succesful initialization")
}

func TestFailIfDockerNotAvailable(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	// Pattern from https://talks.golang.org/2014/testing.slide#1
	if os.Getenv("BE_CRASHER") == "1" {
		endpoint := "unix:///var/run/missing_docker.sock"
		StartDockerClient(endpoint)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestFailIfDockerNotAvailable")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err := cmd.Run()

	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		assert.Equal(t, "exit status 1", err.(*exec.ExitError).Error(), "Expected exit status 1")
		return
	}
	assert.Equal(t, "process ran with err %v", err, "expected exit status 1")
}

func TestCleanImages(t *testing.T) {

	imagesTtl := 10 * time.Hour // Keep images that have been created in the last 10 hours

	hitsPerPath := map[string]int{}
	server := testServer(generateTestData(), &hitsPerPath)
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	cleanedImages := CleanImages(imagesTtl)

	// we should delete two images
	assert.Equal(t, 1, hitsPerPath["/images/4cb07b47f9fb"], "we should be cleaning 4cb07b47f9fb")
	assert.Equal(t, 1, hitsPerPath["/images/5c76a2479c92"], "we should be cleaning 5c76a2479c92")

	// Verify 2 images (12h + week old) were cleaned
	assert.Equal(t, 2, cleanedImages, "we should be removing two images")
	assert.Equal(t, log.InfoLevel, hook.Entries[1].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete image: 4cb07b47f9fb", hook.Entries[0].Message, "expected to delete 4cb07b47f9fb")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete image: 5c76a2479c92", hook.Entries[1].Message, "expected to delete 5c76a2479c92")
}

func TestCleanContainers(t *testing.T) {
	containersTtl := 1 * time.Minute // Keep containers that have exited in past 59seconds

	hitsPerPath := map[string]int{}
	server := testServer(generateTestData(), &hitsPerPath)
	defer server.Close()

	StartDockerClient(server.URL)

	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	cleanedContainers := CleanContainers(containersTtl)

	assert.Equal(t, 1, hitsPerPath["/containers/3176a2479c92"], "we should be cleaning 3176a2479c92")
	assert.Equal(t, 1, hitsPerPath["/containers/4cb07b47f9fb"], "we should be cleaning 4cb07b47f9fb")
	assert.Equal(t, 1, hitsPerPath["/containers/5c76a2479c92"], "we should be cleaning 5c76a2479c92")

	// Verify 2 images (12h + week old) were cleaned
	assert.Equal(t, 3, cleanedContainers, "we should be removing three containers")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[0].Message, "expected to delete 3176a2479c92")
	assert.Equal(t, log.InfoLevel, hook.Entries[1].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete container: 4cb07b47f9fb", hook.Entries[1].Message, "expected to delete 4cb07b47f9fb")
	assert.Equal(t, log.InfoLevel, hook.Entries[2].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete container: 5c76a2479c92", hook.Entries[2].Message, "expected to delete 8dbd9e392a964c")
}

func TestTtlGC(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	containersTtl := 10 * time.Second // Keep containers for 10s
	imagesTtl := 10 * time.Second     // Keep images for 10s

	var interval uint64 = 3 // interval for cron run

	hitsPerPath := map[string]int{}
	server := testServer(generateTestData(), &hitsPerPath)
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	TtlGC(interval, GCPolicy{TtlContainers: containersTtl, TtlImages: imagesTtl})
	// Wait for three runs
	time.Sleep(11 * time.Second)
	StopGC()

	// Assert all that is expected to happen during that 10s period
	assert.Equal(t, 28, len(hook.Entries), "We see 28 message")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "We should use see Info about starting ttl GC")
	assert.Equal(t, "Continous run started in timebased mode with interval (in seconds): 3", hook.Entries[0].Message, "report start of GC")
	assert.Equal(t, "Cleaning all images/containers", hook.Entries[1].Message, "report start of first cleanup")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[2].Message, "clean 12h old image")
	assert.Equal(t, "Trying to delete container: 4cb07b47f9fb", hook.Entries[3].Message, "clean five minutes old image")
	assert.Equal(t, "Trying to delete container: 5c76a2479c92", hook.Entries[4].Message, "Clean old container")
	assert.Equal(t, "Trying to delete image: 3176a2479c92", hook.Entries[5].Message, "clean old image")
	assert.Equal(t, "Trying to delete image: 4cb07b47f9fb", hook.Entries[6].Message, "Clean old image")
	assert.Equal(t, "Cleaning all images/containers", hook.Entries[8].Message, "start of third")
	assert.Equal(t, "Trying to delete container: 9cd87474be90", hook.Entries[9].Message, "Clean old container")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[10].Message, "Clean old container")
}

func TestStatsdReporting(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	hitsPerPath := map[string]int{}
	server := testServer(generateTestData(), &hitsPerPath)
	defer server.Close()

	statsdAddress := "127.0.0.1:6667"

	udp.SetAddr(statsdAddress)
	statsd.Configure(statsdAddress, "test.dockergc.")
	os.Unsetenv("TESTMODE")

	keepLastData := 0 * time.Second // Delete all images

	StartDockerClient(server.URL)

	var cleanedImages int
	var cleanedContainers int

	expectedContainerMessages := []string{
		"test.dockergc.container.dead.amount:5|g",
		"test.dockergc.container.deleted:1|c",
	}
	udp.ShouldReceiveAll(t, expectedContainerMessages, func() {
		cleanedContainers = CleanContainers(keepLastData)
	})

	expectedImageMessages := []string{
		"test.dockergc.image.amount:5|g",
		"test.dockergc.image.deleted:1|c",
	}
	udp.ShouldReceiveAll(t, expectedImageMessages, func() {
		cleanedImages = CleanImages(keepLastData)
	})

	assert.Equal(t, 5, cleanedContainers, "we should be removing five containers")
	assert.Equal(t, 5, cleanedImages, "we should be removing one image")

	os.Setenv("TESTMODE", "true")
}

func TestMonitorDiskSpace(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	hitsPerPath := map[string]int{}
	server := testServer(generateTestData(), &hitsPerPath)
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	fakeDiskSpaceFetcher := &FakeDiskSpaceFetcher{}

	CleanAllWithDiskSpacePolicy(fakeDiskSpaceFetcher, GCPolicy{HighDiskSpaceThreshold: 6, LowDiskSpaceThreshold: 3})

	// Assert that we see 5*10 delete runs for images + 5x100 (all) container deletes + 6*2 info messages (counting down from 9 to 4)
	assert.Equal(t, 72, len(hook.Entries), "We see 72 message")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "We should report starting of cleanup based on threshold")
	assert.Equal(t, "Cleaning images to reach low used disk space threshold", hook.Entries[0].Message, "report low image threshold reached")
}
