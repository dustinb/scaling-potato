package main

import (
	"bytes"
	"github.com/go-redis/redis"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"strconv"
	"time"
)

func main() {

	redisClient := redis.NewClient(&redis.Options{
		Addr: "redis:6379",
	})

	// Increments for every item added to the queue
	var counter = 0
	var speed = 5000

	var queueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "potato",
			Name:      "queue_depth",
			Help:      "Size of the queue",
		}, []string{"name"})

	http.HandleFunc("/speed", func(writer http.ResponseWriter, request *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(request.Body)
		speed, _ = strconv.Atoi(buf.String())
	})

	http.HandleFunc("/add", func(writer http.ResponseWriter, request *http.Request) {
		buf := new(bytes.Buffer)
		buf.ReadFrom(request.Body)
		count, _ := strconv.Atoi(buf.String())
		log.Printf("Adding %d item to the queue", count)
		for i := 0; i < count; i++ {
			counter++
			redisClient.RPush("work", counter)
		}
	})

	// Update the queue_depth metric every 15 seconds
	go func() {
		for {
			time.Sleep(time.Second * 15)
			length, _ := redisClient.LLen("work").Result()
			queueDepth.WithLabelValues("worker").Set(float64(length))
			log.Printf("Queue depth: %d", length)
		}
	}()

	// Do work every speed millisecond
	go func() {
		for {
			time.Sleep(time.Duration(speed) * time.Millisecond)
			item, _ := redisClient.LPop("work").Result()
			log.Printf("Completed work on %v", item)
		}
	}()

	prometheus.MustRegister(queueDepth)
	http.Handle("/metrics", promhttp.Handler())

	_ = http.ListenAndServe(":8011", nil)
}
