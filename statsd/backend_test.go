package statsd

import (
	"bytes"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/segmentio/stats"
)

func TestSetConfigDefaults(t *testing.T) {
	config := setConfigDefaults(Config{})

	if config.Network != "udp" {
		t.Error("invalid default network:", config.Network)
	}

	if config.Address != "localhost:8125" {
		t.Error("invalid default address:", config.Address)
	}
}

func TestProtocol(t *testing.T) {
	tests := []struct {
		metric stats.Metric
		value  interface{}
		rate   float64
		string string
		method func(protocol, io.Writer, stats.Metric, interface{}, float64) error
	}{
		{
			metric: stats.NewGauge(stats.Opts{Name: "hello"}, nil),
			value:  float64(1),
			rate:   float64(1),
			string: "hello:1|g\n",
			method: func(p protocol, w io.Writer, m stats.Metric, v interface{}, r float64) error {
				return p.WriteSet(w, m, v.(float64), r)
			},
		},

		{
			metric: stats.NewCounter(stats.Opts{Name: "hello"}, nil),
			value:  float64(1),
			rate:   float64(0.1),
			string: "hello:1|c|@0.1\n",
			method: func(p protocol, w io.Writer, m stats.Metric, v interface{}, r float64) error {
				return p.WriteAdd(w, m, v.(float64), r)
			},
		},

		{
			metric: stats.NewHistogram(stats.Opts{Name: "hello"}, nil),
			value:  time.Second,
			rate:   float64(1),
			string: "hello:1000|h\n",
			method: func(p protocol, w io.Writer, m stats.Metric, v interface{}, r float64) error {
				return p.WriteObserve(w, m, v.(time.Duration), r)
			},
		},
	}

	for _, test := range tests {
		b := &bytes.Buffer{}
		p := protocol{}

		if err := test.method(p, b, test.metric, test.value, test.rate); err != nil {
			t.Error(err)
		} else if s := b.String(); s != test.string {
			t.Errorf("bad serialization: %#v != %#v", test.string, s)
		}
	}
}

func TestBackend(t *testing.T) {
	packets := []string{}

	server, _ := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	defer server.Close()

	addr := server.LocalAddr()
	join := make(chan struct{})

	go func() {
		defer close(join)
		var b [512]byte

		if n, err := server.Read(b[:]); err != nil {
			t.Error(err)
		} else {
			packets = append(packets, string(b[:n]))
		}
	}()

	a := addr.Network() + "://" + addr.String()
	c := stats.NewClient("statsd", NewBackend(a))
	c.Gauge(stats.Opts{Name: "events,", Unit: "level"}).Set(1)
	c.Counter(stats.Opts{Name: "events", Unit: "count"}).Add(1)
	c.Histogram(stats.Opts{Name: "events", Unit: "duration"}).Observe(time.Second)
	c.Close()

	select {
	case <-join:
	case <-time.After(1 * time.Second):
		t.Error("timeout!")
	}

	if !reflect.DeepEqual(packets, []string{
		`statsd.events_.level:1|g
statsd.events.count:1|c
statsd.events.duration:1000|h
`,
	}) {
		t.Errorf("invalid packets transmitted by the statsd client: %#v", packets)
	}
}