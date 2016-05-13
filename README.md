# Yet another Docker GC

[![Circle CI](https://circleci.com/gh/n1koo/docker-gc.svg?style=svg)](https://circleci.com/gh/n1koo/docker-gc)

Yet another Docker GC but unlike others :

- Written in Go and uses the `go-dockerclient` to talk straight to API (rather than shelling out)
- Actually have tests
- Supports manual cleanups in addition to ttl runs
- Supports reporting errors to [Bugsnag](https://bugsnag.com)
- Supports sending metrics to [dogstatsd](http://docs.datadoghq.com/guides/dogstatsd/)

## Usage

### Compiling
You can compile the binary by doing `script/setup` and `script/compile`. Golang 1.5+ and Git 1.7+ is needed

### Running

```
  docker-gc -command=containers|images|all|emergency [-images_ttl=<DURATION>) [-containers_ttl=<DURATION>]
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc -command=ttl [-interval=<INTERVAL_IN_SECONDS>] [-images_ttl=<DURATION>] [-containers_ttl=<DURATION>] for continuous cleanup based on image/container TTL
  OR
  docker-gc -command=diskspace [-interval=<INTERVAL_IN_SECONDS>] [-high_disk_space_threshold=<PERCENTAGE>] [-low_disk_space_threshold=<PERCENTAGE>] [-containers_ttl=<DURATION>] for continuous cleanup based on used disk space

  You can also specify -bugsnag-key="key" to use bugsnag integration
  and [-statsd_address=<127.0.0.1:815>] and [statsd_namespace=<docker.gc.wtf>] for statsd integration
```

`docker-gc` has two main modes; continuous cleanup and one-time cleanup.

### One-time cleanup

One-time cleanup can be in four ways (as `-command=COMMAND`)

- emergency : clean all containers and images
- all : clean all containers and images but respect `keep_last` values
- images/containers : clean only images or only containers respecting `keep_last` values

eg. `docker-gc -command=all -images_ttl=5m -containers_ttl=1m` would do a one time cleanup of images older than 5minutes and containers older than 1minutes

Default values are:

- `command` = ttl
- `images_ttl` = 10 hours
- `containers_ttl` = 1minutes

### Continuous mode

`docker-gc` has two ttl modes; TTL based and free disk space based. This means the daemon keeps running and does swipes per `interval` settings.

Default value for `interval` is 60 seconds.

### TTL based

eg `docker-gc -command=ttl -interval=5m`

Same TTL settings for containers and images apply than for One-time cleanup

#### Free inode/disk space based

eg `docker-gc -command=diskspace -interval=5m -high_disk_space_threshold=85 -low_disk_space_threshold=50`

Monitors the disk volume used by Docker and if used inodes/disk space hits the `high_disk_space_threshold` threshold starts cleaning up images in batches of 10
until `low_disk_space_threshold` is reached.

NOTICE1: for containers we cleanup based on the `containers_ttl` per `interval because in majority of usecases it makes more senses than looping in batches. 

NOTICE2: The amount in batch might be more than 10 if theres multiple images created at same exact moment (accuracy based on UNIX timestamp)

## Usage

Development can be done on both OSX and Linux. Tests can be run without Docker, but anykind of manual testing requires your user to have rights to `unix:///var/run/docker.sock` (eg. be in `docker` group)

Setup your environment by doing `script/setup`. Tests can be run with `script/test` and running the current version with `script/run` (compile+run+test mode env)