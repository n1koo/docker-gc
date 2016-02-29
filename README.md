# Yet another Docker GC

[![Circle CI](https://circleci.com/gh/n1koo/docker-gc.svg?style=svg)](https://circleci.com/gh/n1koo/docker-gc)

Yet another Docker GC but unlike others :

- Written in Go and uses the `go-dockerclient` to talk straight to API (rather than shelling out)
- Actually have tests
- Supports manual cleanups in addition to continuous runs

## Usage

```
Usage of 'docker-gc':
  docker-gc (-command=containers|images|all|emergency) (-keep_last_images=DURATION) (-keep_last_containers=DURATION)
  -command=all cleans all images and containes respecting keep_last values
  -command=emergency same as all, but with 0second keep_last values
  OR
  docker-gc (-command=continuous) (-interval=INTERVAL_IN_SECONDS) (-keep_last_images=DURATION) (-keep_last_containers=DURATION) for continuous cleanup
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

You an also keep `docker-gc` running in continuous mode which cleans up containers and images periodically. Keep last values are respected and you can specify the interval of which the cleaner should run.


eg `docker-gc -command=continuous -interval=5m` 

Default value for `interval` is 60 seconds