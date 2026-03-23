package main

import (
	"context"
	"container/heap"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	
)

const (
	Attempts int = iota //implements constants incrementally //integer keys
	Retry
)

type Backend struct {
	URL          *url.URL
	Alive        bool
	mux          sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
	ActiveConns int
	index int
}

type ServerPool struct {
	backends ServerHeap
	mux  sync.Mutex
}

func (b *Backend) SetAlive(alive bool) {
	b.mux.Lock()
	b.Alive = alive
	b.mux.Unlock()
}

func (b *Backend) IsAlive() (alive bool) {
	b.mux.RLock()
	alive = b.Alive
	b.mux.RUnlock()
	return
}

func (s *ServerPool) AddBackend(backend *Backend) {
	s.backends = append(s.backends, backend)
}



func (s *ServerPool) GetNextPeer() *Backend {
	s.mux.Lock()
	defer s.mux.Unlock()
	
	if len(s.backends)==0{
		return nil
	}

	best := s.backends[0]
	best.ActiveConns++
	heap.Fix(&s.backends,0)
	return best
}

func (s *ServerPool) MarkBackendStatus(backendUrl *url.URL, alive bool) {
	for _, b := range s.backends {
		if b.URL.String() == backendUrl.String() {
			b.SetAlive(alive)
			break
		}
	}
}

func isBackendAlive(u *url.URL) bool {
	timeout := 2 * time.Second
	conn, err := net.DialTimeout("tcp", u.Host, timeout)
	if err != nil {
		log.Println("site unreachable: ", err)
		return false
	}
	_ = conn.Close()
	return true
}

func (s *ServerPool) HealthCheck() {
	for _, b := range s.backends {
		status := "up"
		alive := isBackendAlive(b.URL)
		b.SetAlive(alive)
		if !alive {
			status = "down"
		}
		log.Printf("%s [%s]\n", b.URL, status)
	}
}

func healthCheck() {
	t := time.NewTicker(time.Second * 20)
	for range t.C {
		log.Println("starting health check ...")
		serverPool.HealthCheck()
		log.Println("health check completed")
	}

}

func GetAttemptsFromContext(r *http.Request) int {
	if attempts, ok := r.Context().Value(Attempts).(int); ok {
		return attempts
	}
	return 0
}

func GetRetryFromContext(r *http.Request) int {
	if retry, ok := r.Context().Value(Retry).(int); ok {
		return retry
	}
	return 0
}

func lb(w http.ResponseWriter, r *http.Request) {

	attempts := GetAttemptsFromContext(r)
	if attempts > 3 {
		log.Printf("%s(%s) Max attempts reached, terminating\n", r.RemoteAddr, r.URL.Path)
		http.Error(w, "Service not available", http.StatusServiceUnavailable)
		return
	}

	if peer := serverPool.GetNextPeer(); peer != nil {
		defer func(){ //load decrement after function is finished
			serverPool.mux.Lock()
			peer.ActiveConns--
			heap.Fix(&serverPool.backends,peer.index)
			serverPool.mux.Unlock()
		}()

		peer.ReverseProxy.ServeHTTP(w, r)
		return
	}
	http.Error(w, "Service not available", http.StatusServiceUnavailable)
}

//heap for sort out alive backends to reduce search surface

type ServerHeap []*Backend

func(h ServerHeap) Len() int{
	return len(h)
}

func ( h ServerHeap) Less(i,j int) bool{ //min heap -> least server load
	return h[i].ActiveConns<h[j].ActiveConns
}

func(h ServerHeap) Swap(i,j int){
	h[i],h[j]=h[j],h[i]
	h[i].index =i
	h[j].index =j
}

func(h *ServerHeap) Push(x any){
	*h = append(*h, x.(*Backend))
}

func (h *ServerHeap) Pop() any{
	old :=*h
	n := len(old)
	item:= old[n-1]
	*h = old[0:n-1]
	return item
}

var serverPool ServerPool

func main() {

	var serverList string
	var port int
	flag.StringVar(&serverList, "backends", "", "Load balanced backends, use commas to separate")
	flag.IntVar(&port, "port", 3030, "Port to server")
	flag.Parse()

	if len(serverList) == 0 {
		log.Fatal("please provide one or more backend to load balance")
	}

	tokens := strings.Split(serverList, ",")
	for _, tok := range tokens {
		serverUrl, err := url.Parse(tok)
		if err != nil {
			log.Fatal(err)
		}

		proxy := httputil.NewSingleHostReverseProxy(serverUrl)

		proxy.ErrorHandler = func(res http.ResponseWriter, req *http.Request, e error) {
			log.Printf("[%s] %s\n", serverUrl.Host, e.Error())
			retries := GetRetryFromContext(req)
			if retries < 3 {
				<-time.After(10 * time.Millisecond) //Receiving from a Channel
				ctx := context.WithValue(req.Context(), Retry, retries+1)
				proxy.ServeHTTP(res, req.WithContext(ctx))
			}

			serverPool.MarkBackendStatus(serverUrl, false)

			attempts := GetAttemptsFromContext(req)
			log.Printf("%s(%s) Attempting retry %d\n", req.RemoteAddr, req.URL.Path, attempts)
			ctx := context.WithValue(req.Context(), Attempts, attempts+1)
			lb(res, req.WithContext(ctx))
		}

		serverPool.AddBackend(&Backend{
			URL:          serverUrl,
			Alive:        true,
			ReverseProxy: proxy,
		})
		log.Printf("configured server: %s\n", serverUrl)

	}

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.HandlerFunc(lb),
	}

	go healthCheck()

	log.Printf("Load Balancer started at :%d\n", port)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}
