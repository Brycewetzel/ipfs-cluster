package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/ipfs-cluster/api"
)

func makeMetric(value string) *api.Metric {
	metr := &api.Metric{
		Name:  "test",
		Peer:  "peer1",
		Value: value,
		Valid: true,
	}
	metr.SetTTL(5 * time.Second)
	return metr
}

func TestNewWindow(t *testing.T) {
	w := NewWindow(10)
	w.window.Next()
}

func TestWindow_Race(t *testing.T) {
	t.SkipNow()
	w := NewWindow(DefaultWindowCap)
	start := make(chan struct{})
	done := make(chan struct{})
	log := make(chan string, 100)

	// go routine to add metrics at regular interval
	addTicker := time.NewTicker(10 * time.Millisecond)
	go func() {
		var i int
		<-start
		for {
			select {
			case <-addTicker.C:
				if i >= 25 {
					i = 0
				}
				time.Sleep(time.Duration(i) * time.Millisecond)
				w.Add(makeMetric("1"))
				i++
			case <-done:
				return
			}
		}
	}()

	// go routine to query latest at regular interval
	latestTicker := time.NewTicker(20 * time.Millisecond)
	go func() {
		<-start
		for {
			select {
			case <-latestTicker.C:
				// l, _ := w.Latest()
				w.Latest()
				// log <- fmt.Sprintf("latest: %v", l)
			case <-done:
				return
			}
		}
	}()

	// go routine to query all at regular interval
	allTicker := time.NewTicker(30 * time.Millisecond)
	go func() {
		<-start
		for {
			select {
			case <-allTicker.C:
				w.All()
				// log <- fmt.Sprintf("all: %v", w.All())
			case <-done:
				return
			}
		}
	}()

	go func() {
		<-start
		<-done
		for s := range log {
			fmt.Println(s)
		}
		close(done)
	}()

	close(start)
	time.Sleep(50 * time.Millisecond)
	done <- struct{}{}
	<-done
}

func TestWindow_Add(t *testing.T) {
	t.Run("add single value", func(t *testing.T) {
		mw := NewWindow(4)
		want := makeMetric("1")
		mw.Add(want)

		mw.wMu.RLock()
		prevRing := mw.window.Prev()
		got, ok := prevRing.Value.(*api.Metric)
		mw.wMu.RUnlock()
		if !ok {
			t.Error("value in window isn't an *api.Metric")
		}

		if got != want {
			t.Errorf("got = %v, want = %v", got, want)
		}
	})
}

func BenchmarkWindow_Add(b *testing.B) {
	b.Run("window size 10", func(b *testing.B) {
		mw := NewWindow(10)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})

	b.Run("window size 25", func(b *testing.B) {
		mw := NewWindow(25)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})

	b.Run("window size 1000", func(b *testing.B) {
		mw := NewWindow(1000)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})
}

func TestWindow_Latest(t *testing.T) {
	t.Run("no metrics error", func(t *testing.T) {
		mw := NewWindow(4)
		_, err := mw.Latest()
		if err != ErrNoMetrics {
			t.Error("expected ErrNoMetrics")
		}
	})

	t.Run("single latest value", func(t *testing.T) {
		mw := NewWindow(4)
		mw.Add(makeMetric("1"))

		metr, err := mw.Latest()
		if err != nil {
			t.Fatal(err)
		}

		if metr.Value != "1" {
			t.Error("expected different value")
		}
	})
}

func BenchmarkWindow_Latest(b *testing.B) {
	b.Run("window size 10", func(b *testing.B) {
		mw := NewWindow(10)
		for i := 0; i < 10; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})

	b.Run("window size 25", func(b *testing.B) {
		mw := NewWindow(25)
		for i := 0; i < 25; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})

	b.Run("window size 1000", func(b *testing.B) {
		mw := NewWindow(1000)
		for i := 0; i < 1000; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.Add(makeMetric("1"))
		}
	})
}

func TestWindow_All(t *testing.T) {
	t.Run("empty window", func(t *testing.T) {
		mw := NewWindow(4)
		if len(mw.All()) != 0 {
			t.Error("expected 0 metrics")
		}
	})

	t.Run("half capacity", func(t *testing.T) {
		mw := NewWindow(4)
		mw.Add(makeMetric("1"))
		mw.Add(makeMetric("2"))

		all := mw.All()
		if len(all) != 2 {
			t.Fatalf("should only be storing 2 metrics: got: %d", len(all))
		}

		if all[0].Value != "2" {
			t.Error("newest metric should be first")
		}

		if all[1].Value != "1" {
			t.Error("older metric should be second")
		}
	})

	t.Run("full capacity", func(t *testing.T) {
		mw := NewWindow(4)
		mw.Add(makeMetric("1"))
		mw.Add(makeMetric("2"))
		mw.Add(makeMetric("3"))
		mw.Add(makeMetric("4"))

		all := mw.All()
		if len(all) != 4 {
			t.Fatalf("should only be storing 4 metrics: got: %d", len(all))
		}

		if all[len(all)-1].Value != "1" {
			t.Error("oldest metric should be 1")
		}
	})

	t.Run("over flow capacity", func(t *testing.T) {
		mw := NewWindow(4)
		mw.Add(makeMetric("1"))
		mw.Add(makeMetric("2"))
		mw.Add(makeMetric("3"))
		mw.Add(makeMetric("4"))
		mw.Add(makeMetric("5"))

		all := mw.All()
		if len(all) != 4 {
			t.Fatalf("should only be storing 4 metrics: got: %d", len(all))
		}

		if all[len(all)-1].Value != "2" {
			t.Error("oldest metric should be 2")
		}

	})
}

func TestWindow_AddParallel(t *testing.T) {
	t.Parallel()

	mw := NewWindow(10)

	t.Run("parallel adder 1", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			mw.Add(makeMetric("adder 1"))
		}
	})

	t.Run("parallel adder 2", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			mw.Add(makeMetric("adder 2"))
		}
	})
}

func BenchmarkWindow_All(b *testing.B) {
	b.Run("window size 10", func(b *testing.B) {
		mw := NewWindow(10)
		for i := 0; i < 10; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.All()
		}
	})

	b.Run("window size 25", func(b *testing.B) {
		mw := NewWindow(25)
		for i := 0; i < 25; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.All()
		}
	})

	b.Run("window size 1000", func(b *testing.B) {
		mw := NewWindow(1000)
		for i := 0; i < 1000; i++ {
			mw.Add(makeMetric("1"))
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			mw.All()
		}
	})
}
