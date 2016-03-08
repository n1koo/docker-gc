package main

import (
	"flag"
	"github.com/stretchr/testify/assert"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestParseFlagsExitsWithBadCommand(t *testing.T) {
	// Pattern from https://talks.golang.org/2014/testing.slide#1
	if os.Getenv("BE_CRASHER") == "1" {
		flag.Set("command", "foobar")
		parseFlags()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestParseFlagsExitsWithBadCommand")
	cmd.Env = append(os.Environ(), "BE_CRASHER=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); ok && !e.Success() {
		assert.Equal(t, "exit status 2", err.(*exec.ExitError).Error(), "Expected exit status 2")
		return
	}
	assert.Equal(t, "process ran with err %v", err, "expected exit status 2")
}

func TestParseFlagsParsesFlags(t *testing.T) {

	// Test containers+values for keep works
	imageDuration := time.Duration(1 * time.Minute)
	containerDuration := time.Duration(5 * time.Hour)
	testCommand := "containers"

	flag.Set("command", testCommand)
	flag.Set("keep_last_images", imageDuration.String())
	flag.Set("keep_last_containers", containerDuration.String())
	parseFlags()

	assert.Equal(t, imageGCPolicy.KeepLastImages, imageDuration, "ImageDuration parsing succeeded")
	assert.Equal(t, imageGCPolicy.KeepLastContainers, containerDuration, "ContainerDuration succeeded")
	assert.Equal(t, command, testCommand, "Command parsing succeeded")
}

func TestParseFlagsWithBadParams(t *testing.T) {
	flag.Set("keep_last_containers", "3")
	parseFlags()
	assert.NotEqual(t, imageGCPolicy.KeepLastContainers.String(), 0, "Command parsing failed")
}
