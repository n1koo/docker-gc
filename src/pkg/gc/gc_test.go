package gc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"pkg/statsd"
	"strconv"
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

var responseIndices = map[string]int{}

type testResponseMap map[string][]string

func testServer(responses testResponseMap) *httptest.Server {
	mux := http.NewServeMux()

	for path, responses := range responses {
		// Variable shadowing.
		_responses := responses

		fun := func(w http.ResponseWriter, r *http.Request) {
			idx := responseIndices[path]
			response := _responses[idx%len(_responses)]
			responseIndices[path] = idx + 1

			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(response))
		}

		mux.Handle(path, http.HandlerFunc(fun))
	}

	server := httptest.NewServer(mux)
	return server
}

func generateFourTestImages() string {
	//Generate 4 images which 2 have been exited in past minute and two havent
	timeNow := time.Now()
	threeSecondsOld := timeNow.Add(-3 * time.Second)
	fiveMinutesOld := timeNow.Add(-5 * time.Minute)
	twelweHoursOld := timeNow.Add(-12 * time.Hour)
	dayOld := timeNow.Add(-24 * time.Hour)

	idsAndDatesMap := make(map[string]string)
	idsAndDatesMap["8dfafdbc3a40"] = strconv.FormatInt(timeNow.Unix(), 10)
	idsAndDatesMap["9cd87474be90"] = strconv.FormatInt(threeSecondsOld.Unix(), 10)
	idsAndDatesMap["3176a2479c92"] = strconv.FormatInt(fiveMinutesOld.Unix(), 10)
	idsAndDatesMap["4cb07b47f9fb"] = strconv.FormatInt(twelweHoursOld.Unix(), 10)
	idsAndDatesMap["5c76a2479c92"] = strconv.FormatInt(dayOld.Unix(), 10)

	var imageList []string

	for id, date := range idsAndDatesMap {
		imageListInfo := `
     {
             "Id":"` + id + `",
             "Created":` + date + `
     }`
		imageList = append(imageList, imageListInfo)
	}

	imageListAsJson := strings.Join(imageList[:], ",")
	imageListAsJson = `[` + imageListAsJson + "\n" + `  ]`

	return imageListAsJson
}

func generateFourTestContainers() (string, map[string]string) {
	//Generate 4 containers of which 2 have been exited in past minute and two havent
	timeNow := time.Now()
	fiveMinutesOld := timeNow.Add(-3 * time.Second)
	twelweHoursOld := timeNow.Add(-12 * time.Hour)
	weekOld := timeNow.Add(-7 * 24 * time.Hour)

	idsAndDatesMap := make(map[string]string)
	idsAndDatesMap["8dfafdbc3a40"] = timeNow.Format(time.RFC3339)
	idsAndDatesMap["9cd87474be90"] = fiveMinutesOld.Format(time.RFC3339)
	idsAndDatesMap["3176a2479c92"] = twelweHoursOld.Format(time.RFC3339)
	idsAndDatesMap["4cb07b47f9fb"] = weekOld.Format(time.RFC3339)

	var containerList []string
	containerListWithFullData := make(map[string]string)

	for id, date := range idsAndDatesMap {
		containerListInfo := `
     {
             "Id":"` + id + `"
     }`
		containerList = append(containerList, containerListInfo)

		containerFullInfo := `
     {
             "Id":"` + id + `",
                   "State": {
                     "Running": false,
                     "FinishedAt": "` + date + `"
                   }
    }`
		containerListWithFullData[id] = containerFullInfo

	}

	containerListAsJson := strings.Join(containerList[:], ",")
	containerListAsJson = `[` + containerListAsJson + "\n" + `  ]`

	return containerListAsJson, containerListWithFullData
}

func generateTestData() testResponseMap {
	imageListAsJson := generateFourTestImages()
	containerListAsJson, containerListWithFullData := generateFourTestContainers()

	responses := testResponseMap{
		"/_ping":           []string{`OK`},
		"/images/":         []string{`OK`},
		"/images/json":     []string{imageListAsJson},
		"/containers/json": []string{containerListAsJson},
	}

	for id, data := range containerListWithFullData {
		responses["/containers/"+id+"/json"] = []string{data}
		responses["/containers/"+id+""] = []string{"ok"}
	}

	return responses
}

func TestStartDockerClient(t *testing.T) {
	responses := map[string][]string{
		"/_ping": []string{"OK"},
	}

	server := testServer(responses)
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
		fmt.Println("DEBUG2")
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

	keepLastImages := 10 * time.Hour // Keep images that have been created in the last 10 hours
	server := testServer(generateTestData())
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	CleanImages(keepLastImages)

	// Verify 2 images (12h + week old) were cleaned
	assert.Equal(t, 2, len(hook.Entries), "we should be removing two images")
	assert.Equal(t, log.InfoLevel, hook.Entries[1].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete image: 4cb07b47f9fb", hook.Entries[0].Message, "expected to delete 4cb07b47f9fb")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete image: 5c76a2479c92", hook.Entries[1].Message, "expected to delete 5c76a2479c92")
}

func TestCleanContainers(t *testing.T) {
	keepLastContainers := 1 * time.Minute // Keep containers that have exited in past 59seconds

	server := testServer(generateTestData())
	defer server.Close()

	StartDockerClient(server.URL)

	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	CleanContainers(keepLastContainers)

	// Verify 2 images (12h + week old) were cleaned
	assert.Equal(t, 2, len(hook.Entries), "we should be removing two images")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete container: 4cb07b47f9fb", hook.Entries[1].Message, "expected to delete 8dbd9e392a964c")
	assert.Equal(t, log.InfoLevel, hook.Entries[1].Level, "all image removal messages should log on Info level")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[0].Message, "expected to delete 8dbd9e392a964c")
}

func TestContinuousGC(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	keepLastContainers := 10 * time.Second // Keep containers for 10s
	keepLastImages := 10 * time.Second     // Keep images for 10s

	var interval uint64 = 3 // interval for cron run

	server := testServer(generateTestData())
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	ContinuousGC(interval, GCPolicy{KeepLastContainers: keepLastContainers, KeepLastImages: keepLastImages})
	// Wait for three runs
	time.Sleep(10 * time.Second)
	StopGC()

	// Assert all that is expected to happen during that 10s period
	assert.Equal(t, 25, len(hook.Entries), "We see 25 message")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "We should use see Info about starting continuous GC")
	assert.Equal(t, "Continous run started in timebased mode with interval (in seconds): 3", hook.Entries[0].Message, "report start of GC")
	assert.Equal(t, "Cleaning all images/containers", hook.Entries[1].Message, "report start of first cleanup")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[2].Message, "clean 12h old image")
	assert.Equal(t, "Trying to delete container: 4cb07b47f9fb", hook.Entries[3].Message, "clean five minutes old image")
	assert.Equal(t, "Trying to delete image: 3176a2479c92", hook.Entries[4].Message, "clean old image")
	assert.Equal(t, "Trying to delete image: 4cb07b47f9fb", hook.Entries[5].Message, "Clean old image")
	assert.Equal(t, "Trying to delete image: 5c76a2479c92", hook.Entries[6].Message, "Clean old image")
	assert.Equal(t, "Cleaning all images/containers", hook.Entries[7].Message, "start of third")
	assert.Equal(t, "Trying to delete container: 9cd87474be90", hook.Entries[8].Message, "Clean old container")
	assert.Equal(t, "Trying to delete container: 3176a2479c92", hook.Entries[9].Message, "Clean old container")
}

func TestStatsdReporting(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	server := testServer(generateTestData())
	defer server.Close()

	statsdAddress := "127.0.0.1:6667"

	udp.SetAddr(statsdAddress)
	statsd.Configure(statsdAddress, "test.dockergc.")
	os.Unsetenv("TESTMODE")

	keepLastData := 0 * time.Second // Delete all images

	StartDockerClient(server.URL)

	expectedContainerMessages := []string{
		"test.dockergc.container.dead.amount:4|g",
		"test.dockergc.container.deleted:1|c",
	}
	udp.ShouldReceiveAll(t, expectedContainerMessages, func() {
		CleanContainers(keepLastData)
	})

	expectedImageMessages := []string{
		"test.dockergc.image.amount:5|g",
		"test.dockergc.image.deleted:1|c",
	}
	udp.ShouldReceiveAll(t, expectedImageMessages, func() {
		CleanImages(keepLastData)
	})

	os.Setenv("TESTMODE", "true")
}

func TestMonitorDiskSpace(t *testing.T) {
	_, hook := logrustest.NewNullLogger()
	log.AddHook(hook)

	server := testServer(generateTestData())
	defer server.Close()

	Client = nil
	StartDockerClient(server.URL)

	fakeDiskSpaceFetcher := &FakeDiskSpaceFetcher{}

	CleanAllWithDiskSpacePolicy(fakeDiskSpaceFetcher, GCPolicy{HighDiskSpaceThreshold: 6, LowDiskSpaceThreshold: 3})

	// Assert that we see 6*10 delete runs for images + 6x100 (all) container deletes + 6*2 info messages (counting down from 9 to 4)
	assert.Equal(t, 66, len(hook.Entries), "We see 67 message")
	assert.Equal(t, log.InfoLevel, hook.Entries[0].Level, "We should report starting of cleanup based on threshold")
	assert.Equal(t, "Cleaning images to reach low used disk space threshold", hook.Entries[0].Message, "report low image threshold reached")
}
