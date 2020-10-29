package main

import (
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "net/http"
    "os"
    "os/signal"
    "time"
    "bytes"
    "io/ioutil"
    "encoding/json"
    "log"
    "sync/atomic"
    "context"
    "flag"
)

var (
    sent *prometheus.Desc = prometheus.NewDesc("mandrill_sent_total", "Total number of sent mails.", []string{"tag"}, nil)
    hardBounces *prometheus.Desc = prometheus.NewDesc("mandrill_hard_bounces", "Number of mails bounced hard", []string{"tag"}, nil)
    softBounces *prometheus.Desc = prometheus.NewDesc("mandrill_soft_bounces", "Number of mails bounced soft", []string{"tag"}, nil)
    rejects *prometheus.Desc = prometheus.NewDesc("mandrill_rejects", "Number of mails rejected", []string{"tag"}, nil)
    complaints *prometheus.Desc = prometheus.NewDesc("mandrill_complaints", "Number of complaints", []string{"tag"}, nil)
    unsubs *prometheus.Desc = prometheus.NewDesc("mandrill_unsubs", "Number of unsubscribes", []string{"tag"}, nil)
    opens *prometheus.Desc = prometheus.NewDesc("mandrill_opens", "Number of mails opened", []string{"tag"}, nil)
    clicks *prometheus.Desc = prometheus.NewDesc("mandrill_clicks", "Number of clicks inside mails", []string{"tag"}, nil)
    unique_opens *prometheus.Desc = prometheus.NewDesc("mandrill_unique_opens", "Unique number of mails opened", []string{"tag"}, nil)
    unique_clicks *prometheus.Desc = prometheus.NewDesc("mandrill_unique_clicks", "Unique number of clicks", []string{"tag"}, nil)
    reputation *prometheus.Desc = prometheus.NewDesc("mandrill_reputation", "Mandrill reputation", []string{"tag"}, nil)
    listenAddr string
    healthy    int32
)

type mandrillCollector struct {
    apiKey string
}

func (m mandrillCollector) Describe(ch chan <- *prometheus.Desc) {
    ch <- sent
    ch <- hardBounces
    ch <- softBounces
    ch <- rejects
    ch <- complaints
    ch <- unsubs
    ch <- opens
    ch <- clicks
    ch <- unique_opens
    ch <- unique_clicks
    ch <- reputation
}

type mandrillTagData struct {
    Tag           string
    Sent          int
    SoftBounces   int  `json:"soft_bounces"`
    HardBounces   int  `json:"hard_bounces"`
    Rejects       int
    Complaints    int
    Unsubs        int
    Opens         int
    Clicks        int
    Unique_opens  int
    Unique_clicks int
    Reputation    int
}

func (m mandrillCollector) Collect(ch chan <- prometheus.Metric) {

    //get Tags from Mandrill
    tagData, err := m.getTags()
    if err != nil {
        log.Print(err)
    }

    //iterate over tags and get stats
    for _, tag := range tagData {
        ch <- prometheus.MustNewConstMetric(sent, prometheus.CounterValue, float64(tag.Sent), tag.Tag)
        ch <- prometheus.MustNewConstMetric(hardBounces, prometheus.CounterValue, float64(tag.HardBounces), tag.Tag)
        ch <- prometheus.MustNewConstMetric(softBounces, prometheus.CounterValue, float64(tag.SoftBounces), tag.Tag)
        ch <- prometheus.MustNewConstMetric(rejects, prometheus.CounterValue, float64(tag.Rejects), tag.Tag)
        ch <- prometheus.MustNewConstMetric(complaints, prometheus.CounterValue, float64(tag.Complaints), tag.Tag)
        ch <- prometheus.MustNewConstMetric(unsubs, prometheus.CounterValue, float64(tag.Unsubs), tag.Tag)
        ch <- prometheus.MustNewConstMetric(opens, prometheus.CounterValue, float64(tag.Opens), tag.Tag)
        ch <- prometheus.MustNewConstMetric(clicks, prometheus.CounterValue, float64(tag.Clicks), tag.Tag)
        ch <- prometheus.MustNewConstMetric(unique_opens, prometheus.CounterValue, float64(tag.Unique_opens), tag.Tag)
        ch <- prometheus.MustNewConstMetric(unique_clicks, prometheus.CounterValue, float64(tag.Unique_clicks), tag.Tag)
        ch <- prometheus.MustNewConstMetric(reputation, prometheus.CounterValue, float64(tag.Reputation), tag.Tag)
    }
}

func (m mandrillCollector) getTags() ([]mandrillTagData, error) {

    body := bytes.Buffer{}
    body.WriteString("{\"key\": \"")
    body.WriteString(m.apiKey)
    body.WriteString("\"}")

    client := &http.Client{}
    req, err := http.NewRequest("POST", "https://mandrillapp.com/api/1.0/tags/list.json", &body)
    req.Header.Add("Content-Type", "application/json; charset=utf-8")
    resp, err := client.Do(req)
    if err != nil {
        return nil, err
    }
    respBody, _ := ioutil.ReadAll(resp.Body)

    result := []mandrillTagData{}
    err = json.Unmarshal(respBody, &result)
    if err != nil {
        return nil, err
    }

    return result, nil
}

func main() {
    mc := mandrillCollector{
        apiKey:os.Getenv("MANDRILL_API_KEY"),
    }

    flag.StringVar(&listenAddr, "listen-addr", ":9153", "server listen address")
    flag.Parse()
    prometheus.MustRegister(mc)

    router := http.NewServeMux()
    router.Handle("/", index())
    router.Handle("/healthz", healthz())
    router.Handle("/metrics", promhttp.Handler())

    logger := log.New(os.Stdout, "http: ", log.LstdFlags)
    logger.Println("Server is starting...")

    server := &http.Server{
        Addr:         listenAddr,
        Handler:      (logging(logger)(router)),
        ErrorLog:     logger,
        ReadTimeout:  5 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  15 * time.Second,
    }

    done := make(chan bool)
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, os.Interrupt)

    go func() {
        <-quit
        logger.Println("Server is shutting down...")
        atomic.StoreInt32(&healthy, 0)

        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()

        server.SetKeepAlivesEnabled(false)
        if err := server.Shutdown(ctx); err != nil {
            logger.Fatalf("Could not gracefully shutdown the server: %v\n", err)
        }
        close(done)
    }()

    logger.Println("Server is ready to handle requests at", listenAddr)
    atomic.StoreInt32(&healthy, 1)
    //default Seite
    // http.Handle("/metrics", promhttp.Handler())
    //port 9153 https://github.com/prometheus/prometheus/wiki/Default-port-allocations
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("Could not listen on :9153: %v\n", err)
    }

    <-done
    logger.Println("Server stopped")
}

func index() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path != "/" {
            http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
            return
        }
        w.Header().Set("Content-Type", "text/plain; charset=utf-8")
        w.WriteHeader(http.StatusOK)
        w.Write([]byte(`<html>
             <head><title>Mandrill statistics Exporter</title></head>
             <body>
             <h1>Madrill statistics Exporter</h1>
             <p><a href='metrics'>Metrics</a></p>
             </body>
             </html>`))
    })
}

func healthz() http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if atomic.LoadInt32(&healthy) == 1 {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        w.WriteHeader(http.StatusServiceUnavailable)
    })
}

func logging(logger *log.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                logger.Println(r.Method, r.URL.Path, r.RemoteAddr, r.UserAgent())
            }()
            next.ServeHTTP(w, r)
        })
    }
}
