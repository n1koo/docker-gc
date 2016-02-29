package main

import (
	"flag"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
	t.Fatalf("process ran with err %v, want exit status 2", err)
}

func TestParseFlagsParsesFlags(t *testing.T) {

	imageDuration := time.Duration(1 * time.Minute)
	containerDuration := time.Duration(5 * time.Hour)
	command := "containers"

	flag.Set("command", command)
	flag.Set("keep_last_images", imageDuration.String())
	flag.Set("keep_last_containers", containerDuration.String())
	parseFlags()

	assert.Equal(t, KeepLastImages, imageDuration, "ImageDuration parsing failed")
	assert.Equal(t, KeepLastContainers, containerDuration, "ContainerDuration parsing failed")
	assert.Equal(t, Command, command, "Command parsing failed")
}

func TestParseFlagsWithBadParams(t *testing.T) {
	flag.Set("keep_last_containers", "3")
	parseFlags()
	assert.NotEqual(t, KeepLastContainers.String(), 0, "Command parsing failed")
}
