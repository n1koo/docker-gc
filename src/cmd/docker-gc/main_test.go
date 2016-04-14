package main

import (
	"flag"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseFlagsParsesFlags(t *testing.T) {

	// Test containers+values for keep works
	imageDuration := time.Duration(1 * time.Minute)
	containerDuration := time.Duration(5 * time.Hour)
	testCommand := "containers"

	flag.Set("command", testCommand)
	flag.Set("images_ttl", imageDuration.String())
	flag.Set("containers_ttl", containerDuration.String())
	parseFlags()

	assert.Equal(t, gcPolicy.TtlImages, imageDuration, "ImageDuration parsing didn't succeed")
	assert.Equal(t, gcPolicy.TtlContainers, containerDuration, "ContainerDuration didn't succeed")
	assert.Equal(t, command, testCommand, "Command parsing didn't succeed")
}

func TestParseFlagsWithBadParams(t *testing.T) {
	flag.Set("containers_ttl", "3")
	parseFlags()
	assert.NotEqual(t, gcPolicy.TtlContainers.String(), 0, "Command parsing failed")
}
