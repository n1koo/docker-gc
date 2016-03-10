# Yet another Docker GC

[![Circle CI](https://circleci.com/gh/n1koo/docker-gc.svg?style=svg)](https://circleci.com/gh/n1koo/docker-gc)

Yet another Docker GC but unlike others :

- Written in Go and uses the `go-dockerclient` to talk straight to API (rather than shelling out)
- Actually have tests
- Supports manual cleanups in addition to continuous runs
- Supports reporting errors to [Bugsnag](https://bugsnag.com)
- Supports sending metrics to [dogstatsd](http://docs.datadoghq.com/guides/dogstatsd/)

## Usage

### Compiling
You can compile the binary by doing `script/setup` and `script/compile`. Golang 1.5+ and Git 1.7+ is needed

### Running

```
  docker-gc (-command=containers|images|all|emergency) (-keep_last_images=DURATION) (-keep_last_containers=DURATION)
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc (-command=continuous) (-interval=INTERVAL_IN_SECONDS) (-keep_last_images=DURATION) (-keep_last_containers=DURATION) for continuous cleanup in TTL mode
  OR
  docker-gc (-command=diskspace) (-interval=INTERVAL_IN_SECONDS) (-high_disk_space_threshold=PERCENTAGE) (-low_disk_space_threshold=PERCENTAGE) for disk space based continuous mode

  You can also specify -bugsnag-key="key" to use bugsnag integration
  and -statsd_address=127.0.0.1:815 and statsd_namespace=docker.gc.wtf. for statsd integration
```

`docker-gc` has two main modes; continuous cleanup and one-time cleanup. 

### One-time cleanup

One-time cleanup can be in four ways (as `-command=COMMAND`)

- emergency : clean all containers and images
- all : clean all containers and images but respect `keep_last` values
- images/containers : clean only images or only containers respecting `keep_last` values

eg. `docker-gc -command=all -keep_last_images=5m -keep_last_containers=1m` would do a one time cleanup of images older than 5minutes and containers older than 1minutes

Default values are: 

- `command` = continuous
- `keep_last_images` = 10 hours
- `keep_last_containers` = 1minutes

### Continuous mode

`docker-gc` has two continuous modes; TTL based and free disk space based. This means the daemon keeps running and does swipes per `interval` settings.

Default value for `interval` is 60 seconds. 

### TTL based

eg `docker-gc -command=continuous -interval=5m` 

Same TTL settings for containers and images apply than for One-time cleanup

#### Free disk space based

eg `docker-gc -command=diskspace -interval=5m -high_disk_space_threshold=85 -low_disk_space_threshold=50`

Monitors the disk volume used by Docker and if used disk space hits the `high_disk_space_threshold` threshold starts cleaning up containers and images in batches of 10
until `low_disk_space_threshold` is reached.

NOTICE: for containers we cleanup based on the `keep_last_containers`  because in majority of usecases it makes more senses than looping in batches. 
For images we do this in patches of 10 starting from the oldest after that.

## Usage

Development can be done on both OSX and Linux. Tests can be run without Docker, but anykind of manual testing requires your user to have rights to `unix:///var/run/docker.sock` (eg. be in `docker` group)

Setup your environment by doing `script/setup`. Tests can be run with `script/test` and running the current version with `script/run` (compile+run+test mode env)