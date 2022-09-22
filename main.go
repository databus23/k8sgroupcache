package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/databus23/k8sgroupcache/k8spool"
	"github.com/databus23/k8sgroupcache/metrics"

	"github.com/mailgun/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

var (
	selfip    string
	namespace string
	selector  string
	port      int
)

func init() {

	// Only log the warning severity or above.
	logrus.SetLevel(logrus.DebugLevel)
	groupcache.SetLogger(logrus.StandardLogger().WithField("module", "groupcache"))
}

func main() {

	flag.StringVar(&selfip, "selfip", os.Getenv("POD_IP"), "own ip")
	flag.StringVar(&namespace, "namespace", os.Getenv("POD_NAMESPACE"), "pod namespace")
	flag.StringVar(&selector, "selector", os.Getenv("SELECTOR"), "selector")
	flag.IntVar(&port, "port", 8080, "port")
	flag.Parse()

	localpeer := fmt.Sprintf("http://%s:%d", selfip, port)
	log.Printf("localpeer: %s", localpeer)

	pool := groupcache.NewHTTPPoolOpts(localpeer, &groupcache.HTTPPoolOptions{})

	log.Printf("Starting k8s cache pool watcher with selector %s...", selector)
	_, err := k8spool.New(k8spool.Config{
		PeerScheme: "http",
		PeerPort:   port,
		Namespace:  namespace,
		Selector:   selector,
		OnUpdate: func(peers []string) {
			log.Printf("update cache peers: %v", peers)
			pool.Set(peers...)
		},
	})

	if err != nil {
		log.Fatalf("Failed to start k8s peer watcher: %s", err)
	}

	group := groupcache.NewGroup("testgroup", 3000000, groupcache.GetterFunc(
		func(ctx context.Context, id string, dest groupcache.Sink) error {
			time.Sleep(5 * time.Second)
			return dest.SetString(fmt.Sprintf("Value %s calculated by %s", id, selfip), time.Time{})
		},
	))

	reg := prometheus.NewRegistry()
	reg.Register(metrics.NewGroupCollector(group))

	mux := http.NewServeMux()
	mux.Handle("/_groupcache/", pool)
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health/ready", func(rw http.ResponseWriter, _ *http.Request) {
		rw.Write([]byte("ok"))
	})
	mux.Handle("/", &server{group: group})
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}

	log.Println("Listening on ", server.Addr)
	server.ListenAndServe()

}

type server struct {
	group *groupcache.Group
}

func (s *server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	var result string
	s.group.Get(req.Context(), strings.TrimPrefix(req.URL.Path, "/"), groupcache.StringSink(&result))

	rw.Header().Add("Content-Type", "text/plain")
	rw.WriteHeader(200)
	rw.Write([]byte(fmt.Sprintf("Server: %s\nValue from cache: %s\n", selfip, result)))

}
