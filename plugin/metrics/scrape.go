package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/matttproud/golang_protobuf_extensions/pbutil"
	"github.com/prometheus/common/expfmt"

	dto "github.com/prometheus/client_model/go"
)

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3`

// Family mirrors the MetricFamily proto message.
type Family struct {
	//Time    time.Time
	Name    string        `json:"name"`
	Help    string        `json:"help"`
	Type    string        `json:"type"`
	Metrics []interface{} `json:"metrics,omitempty"` // Either metric or summary.
}

// Metric is for all "single value" metrics, i.e. Counter, Gauge, and Untyped.
type Metric struct {
	Labels      map[string]string `json:"labels,omitempty"`
	TimestampMs string            `json:"timestamp_ms,omitempty"`
	Value       string            `json:"value"`
}

// Summary mirrors the Summary proto message.
type Summary struct {
	Labels      map[string]string `json:"labels,omitempty"`
	TimestampMs string            `json:"timestamp_ms,omitempty"`
	Quantiles   map[string]string `json:"quantiles,omitempty"`
	Count       string            `json:"count"`
	Sum         string            `json:"sum"`
}

// Histogram mirrors the Histogram proto message.
type Histogram struct {
	Labels      map[string]string `json:"labels,omitempty"`
	TimestampMs string            `json:"timestamp_ms,omitempty"`
	Buckets     map[string]string `json:"buckets,omitempty"`
	Count       string            `json:"count"`
	Sum         string            `json:"sum"`
}

// FetchMetricFamilies is used to fetch metrics for the given URL
func FetchMetricFamilies(url string, ch chan<- *dto.MetricFamily) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("creating GET request for URL %q failed: %v", url, err)
	}
	req.Header.Add("Accept", acceptHeader)
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("executing GET request for URL %q failed: %v", url, err)
	}
	return ParseResponse(resp, ch)
}

// ParseResponse is used to parse response from the url
func ParseResponse(resp *http.Response, ch chan<- *dto.MetricFamily) error {
	mediatype, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err == nil && mediatype == "application/vnd.google.protobuf" &&
		params["encoding"] == "delimited" &&
		params["proto"] == "io.prometheus.client.MetricFamily" {
		defer close(ch)
		for {
			mf := &dto.MetricFamily{}
			if _, err = pbutil.ReadDelimited(resp.Body, mf); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("reading metric family protocol buffer failed: %v", err)
			}
			ch <- mf
		}
	} else {
		if err := ParseReader(resp.Body, ch); err != nil {
			return err
		}
	}
	return nil
}

// ParseReader consumes an io.Reader and pushes it to the MetricFamily
// channel. It returns when all MetricFamilies are parsed and put on the
// channel.
func ParseReader(in io.Reader, ch chan<- *dto.MetricFamily) error {
	defer close(ch)
	var parser expfmt.TextParser
	metricFamilies, err := parser.TextToMetricFamilies(in)
	if err != nil {
		return fmt.Errorf("reading text format failed: %v", err)
	}
	for _, mf := range metricFamilies {
		ch <- mf
	}
	return nil
}

// NewFamily consumes a MetricFamily and transforms it to the local Family type.
func NewFamily(dtoMF *dto.MetricFamily) *Family {
	mf := &Family{
		//Time:    time.Now(),
		Name:    dtoMF.GetName(),
		Help:    dtoMF.GetHelp(),
		Type:    dtoMF.GetType().String(),
		Metrics: make([]interface{}, len(dtoMF.Metric)),
	}
	for i, m := range dtoMF.Metric {
		if dtoMF.GetType() == dto.MetricType_SUMMARY {
			mf.Metrics[i] = Summary{
				Labels:      makeLabels(m),
				TimestampMs: makeTimestamp(m),
				Quantiles:   makeQuantiles(m),
				Count:       fmt.Sprint(m.GetSummary().GetSampleCount()),
				Sum:         fmt.Sprint(m.GetSummary().GetSampleSum()),
			}
		} else if dtoMF.GetType() == dto.MetricType_HISTOGRAM {
			mf.Metrics[i] = Histogram{
				Labels:      makeLabels(m),
				TimestampMs: makeTimestamp(m),
				Buckets:     makeBuckets(m),
				Count:       fmt.Sprint(m.GetHistogram().GetSampleCount()),
				Sum:         fmt.Sprint(m.GetHistogram().GetSampleSum()),
			}
		} else {
			mf.Metrics[i] = Metric{
				Labels:      makeLabels(m),
				TimestampMs: makeTimestamp(m),
				Value:       fmt.Sprint(getValue(m)),
			}
		}
	}
	return mf
}

func getValue(m *dto.Metric) float64 {
	if m.Gauge != nil {
		return m.GetGauge().GetValue()
	}
	if m.Counter != nil {
		return m.GetCounter().GetValue()
	}
	if m.Untyped != nil {
		return m.GetUntyped().GetValue()
	}
	return 0.
}

func makeLabels(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, lp := range m.Label {
		result[lp.GetName()] = lp.GetValue()
	}
	return result
}

func makeTimestamp(m *dto.Metric) string {
	if m.TimestampMs == nil {
		return ""
	}
	return fmt.Sprint(m.GetTimestampMs())
}

func makeQuantiles(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, q := range m.GetSummary().Quantile {
		result[fmt.Sprint(q.GetQuantile())] = fmt.Sprint(q.GetValue())
	}
	return result
}

func makeBuckets(m *dto.Metric) map[string]string {
	result := map[string]string{}
	for _, b := range m.GetHistogram().Bucket {
		result[fmt.Sprint(b.GetUpperBound())] = fmt.Sprint(b.GetCumulativeCount())
	}
	return result
}

// ScrapeMetrics is used to finally scrape metrics
func ScrapeMetrics(url string) {
	mfChan := make(chan *dto.MetricFamily, 1024)
	go func() {
		err := FetchMetricFamilies(url, mfChan)
		if err != nil {
			log.Fatal(err)
		}
	}()

	result := []*Family{}
	for mf := range mfChan {
		result = append(result, NewFamily(mf))
	}
	jsonText, err := json.Marshal(result)
	if err != nil {
		log.Fatal("error marshaling JSON:", err)
	}
	if _, err := os.Stdout.Write(jsonText); err != nil {
		log.Fatal("error writing to stdout:", err)
	}
	fmt.Println()
}
