// Package statsd contains a singleton statsd client for use in all other
// packages. It can be configured once at application startup, and imported by
// any package that wishes to record metrics.
package statsd

import (
  "errors"
  "time"
  "os"
  "github.com/Sirupsen/logrus"
  "github.com/Shopify/go-dogstatsd"
)

var (
  // Statsd is a globally-shared datadog client. It is configured once by the
  // Configure function, then used by all the other functions of this package.
  Statsd *dogstatsd.Client
)

var errNotConfigured = errors.New("statsd is not configured")

// Configure should be called once, before any metrics are submitted, with the
// statsd endpoint to submit to.
func Configure(endpoint, namespace string) (err error) {
  Statsd, err = dogstatsd.New(endpoint, &dogstatsd.Context{
    Namespace: namespace,
    })
    
  if err != nil {
    logrus.WithFields(logrus.Fields{
      "action":   "statsd_dial",
      "endpoint": endpoint,
      "error":    err,
    }).Warn("Unable to dial StatsD.")
    return err
  }

  return nil
}

// Count submits a Count metric to the global Statsd instance, if configured.
// See go-dogstatsd for more documentation on Count.
func Count(m string, n int64, ts []string, r float64) {
  if Statsd == nil {
    puke(errNotConfigured)
    return
  }
  if err := Statsd.Count(m, n, ts, r); err != nil {
    puke(err)
  }
}

// Event submits an Event to the global Statsd instance, if configured. See
// go-dogstatsd for more documentation on Event.
func Event(a, b string, c []string) {
  if Statsd == nil {
    puke(errNotConfigured)
    return
  }
  if err := Statsd.Event(a, b, c); err != nil {
    puke(err)
  }
}

// Timer submits a Timer metric to the global Statsd instance, if configured.
// See go-dogstatsd for more documentation on Timer.
func Timer(m string, n time.Duration, ts []string, r float64) {
  if Statsd == nil {
    puke(errNotConfigured)
    return
  }
  if err := Statsd.Timer(m, n, ts, r); err != nil {
    puke(err)
  }
}

// Gauge submits a Gauge metric to the global Statsd instance, if configured.
func Gauge(metric string, n int) {
  if Statsd == nil {
    puke(errNotConfigured)
    return
  }
  if err := Statsd.Gauge(metric, float64(n), []string{}, 1); err != nil {
    puke(err)
  }
}

func puke(err error) {
  if os.Getenv("TESTMODE") == "" {
    logrus.WithField("error", err).Warn("couldn't submit event to statsd")
  }
}