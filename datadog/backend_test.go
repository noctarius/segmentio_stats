package datadog

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
		value  float64
		string string
		method func(protocol, io.Writer, stats.Metric, float64, time.Time) error
	}{
		{
			metric: stats.NewGauge(stats.Opts{Name: "hello"}),
			value:  1,
			string: "hello:1|g\n",
			method: protocol.WriteSet,
		},

		{
			metric: stats.NewCounter(stats.Opts{Name: "hello", Sample: 0.1}),
			value:  1,
			string: "hello:1|c|@0.1\n",
			method: protocol.WriteAdd,
		},

		{
			metric: stats.NewHistogram(stats.Opts{Name: "hello"}),
			value:  1,
			string: "hello:1|h\n",
			method: protocol.WriteObserve,
		},
	}

	for _, test := range tests {
		now := time.Unix(1, 0)
		b := &bytes.Buffer{}
		p := protocol{}

		if err := test.method(p, b, test.metric, test.value, now); err != nil {
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
	c := stats.NewClient(NewBackend(a),
		stats.Tag{Name: "hello:", Value: "world,"},
		stats.Tag{Name: "answer", Value: "42"},
	)

	c.Gauge("events.level").Set(1)
	c.Counter("events.count").Add(1)
	c.Histogram("events.seconds").Observe(1)
	c.Close()

	select {
	case <-join:
	case <-time.After(1 * time.Second):
		t.Error("timeout!")
	}

	if !reflect.DeepEqual(packets, []string{
		`datadog.test.events.level:1|g|#hello_:world_,answer:42
datadog.test.events.count:1|c|#hello_:world_,answer:42
datadog.test.events.seconds:1|h|#hello_:world_,answer:42
`,
	}) {
		t.Errorf("invalid packets transmitted by the datadog client: %#v", packets)
	}
}
