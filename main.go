package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const supportedWebhookVersion = "4"

var (
	addr        = "localhost:9097"
	application = "alertmanager2es"
	// See AlertManager docs for info on alert groupings:
	// https://prometheus.io/docs/alerting/configuration/#route-<route>
	esType = "alert_group"
	// Index by month as we don't produce enough data to warrant a daily index
	esIndexDateFormat = "2006.01"
	esIndexName       = "alertmanager"
	esURL             string
	revision          = "unknown"
	versionString     = fmt.Sprintf("%s %s (%s)", application, revision, runtime.Version())

	notificationsErrored = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: application,
		Name:      "notifications_errored_total",
		Help:      "Total number of alert notifications that errored during processing and should be retried",
	})
	notificationsInvalid = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: application,
		Name:      "notifications_invalid_total",
		Help:      "Total number of invalid alert notifications received",
	})
	notificationsReceived = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: application,
		Name:      "notifications_received_total",
		Help:      "Total number of alert notifications received",
	})
)

func init() {
	prometheus.MustRegister(notificationsErrored)
	prometheus.MustRegister(notificationsInvalid)
	prometheus.MustRegister(notificationsReceived)
}

func main() {
	var showVersion bool
	flag.StringVar(&addr, "addr", addr, "host:port to listen to")
	flag.StringVar(&esIndexDateFormat, "esIndexDateFormat", esIndexDateFormat, "Elasticsearch index date format")
	flag.StringVar(&esIndexName, "esIndexName", esIndexName, "Elasticsearch index name")
	flag.StringVar(&esType, "esType", esType, "Elasticsearch document type ('_type')")
	flag.StringVar(&esURL, "esURL", esURL, "Elasticsearch HTTP URL")
	flag.BoolVar(&showVersion, "version", false, "Print version number and exit")
	flag.Parse()
	
	if showVersion {
		fmt.Println(versionString)
		os.Exit(0)
	}

	if esURL == "" {
		fmt.Fprintln(os.Stderr, "Must specify HTTP URL for Elasticsearch")
		flag.Usage()
		os.Exit(2)
	}

	http.DefaultClient.Timeout = 10 * time.Second
	s := &http.Server{
		Addr:         addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, versionString)
	})
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/webhook", prometheus.InstrumentHandlerFunc("webhook", http.HandlerFunc(handler)))

	log.Print(versionString)
	log.Printf("Listening on %s", addr)
	log.Fatal(s.ListenAndServe())
}

func handler(w http.ResponseWriter, r *http.Request) {
	notificationsReceived.Inc()

	if r.Body == nil {
		notificationsInvalid.Inc()
		err := errors.New("got empty request body")
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		notificationsErrored.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
	defer r.Body.Close()

	var msg notification
	err = json.Unmarshal(b, &msg)
	if err != nil {
		notificationsInvalid.Inc()
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	if msg.Version != supportedWebhookVersion {
		notificationsInvalid.Inc()
		err := fmt.Errorf("Do not understand webhook version %q, only version %q is supported.", msg.Version, supportedWebhookVersion)
		http.Error(w, err.Error(), http.StatusBadRequest)
		log.Print(err)
		return
	}

	now := time.Now()
	// ISO8601: https://github.com/golang/go/issues/2141#issuecomment-66058048
	msg.Timestamp = now.Format(time.RFC3339)

	index := fmt.Sprintf("%s-%s/%s", esIndexName, now.Format(esIndexDateFormat), esType)
	url := fmt.Sprintf("%s/%s", esURL, index)

	b, err = json.Marshal(&msg)
	if err != nil {
		notificationsErrored.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
	if err != nil {
		notificationsErrored.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	req.Header.Set("User-Agent", versionString)
	req.Header.Set("Content-Type", "application/json")

	esUser := os.Getenv("ES_USER")
	esPass := os.Getenv("ES_PASS")

	if len(esUser) != 0 && len(esPass) != 0 {
		req.SetBasicAuth(esUser, esPass)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		notificationsErrored.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	body, err := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		notificationsErrored.Inc()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}

	if resp.StatusCode/100 != 2 {
		notificationsErrored.Inc()
		err := fmt.Errorf("POST to Elasticsearch on %q returned HTTP %d:  %s", url, resp.StatusCode, body)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Print(err)
		return
	}
}

type notification struct {
	Alerts []struct {
		Annotations  map[string]string `json:"annotations"`
		EndsAt       time.Time         `json:"endsAt"`
		GeneratorURL string            `json:"generatorURL"`
		Labels       map[string]string `json:"labels"`
		StartsAt     time.Time         `json:"startsAt"`
		Status       string            `json:"status"`
	} `json:"alerts"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	CommonLabels      map[string]string `json:"commonLabels"`
	ExternalURL       string            `json:"externalURL"`
	GroupLabels       map[string]string `json:"groupLabels"`
	Receiver          string            `json:"receiver"`
	Status            string            `json:"status"`
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`

	// Timestamp records when the alert notification was received
	Timestamp string `json:"@timestamp"`
}
