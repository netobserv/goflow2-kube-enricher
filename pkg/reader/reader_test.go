package reader

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var spy = SpyDriver{shutdownCalled: false, nextCalled: false}

type SpyDriver struct {
	nextCalled     bool
	shutdownCalled bool
}

type TestDriver struct{}

func (gf *TestDriver) Next() (map[string]interface{}, error) {
	records := map[string]interface{}{
		"SrcAddr": "10.0.0.1",
		"DstAddr": "10.0.0.2",
	}
	spy.nextCalled = true
	return records, nil
}

func (gf *TestDriver) Shutdown() {
	spy.shutdownCalled = true
}

func setupSimpleReader() *Reader {
	log := logrus.NewEntry(logrus.New())
	r := Reader{
		format:   &TestDriver{},
		log:      log,
		enricher: nil,
	}
	return &r
}

func TestShutdown(t *testing.T) {
	r := setupSimpleReader()

	ctx, cancel := context.WithCancel(context.TODO())
	go r.Start(ctx)

	//check if next has been called and ensure shutdown has not been called, then cancel
	assert.Eventually(t, func() bool { return spy.nextCalled }, time.Second, time.Millisecond)
	assert.Equal(t, false, spy.shutdownCalled)
	cancel()

	//check if shutdown has been called then reset spy
	time.Sleep(time.Millisecond)
	assert.Eventually(t, func() bool { return spy.shutdownCalled }, time.Second, time.Millisecond)
	spy = SpyDriver{shutdownCalled: false, nextCalled: false}

	//check that shutdown and next are not called anymore
	time.Sleep(time.Millisecond)
	assert.Eventually(t, func() bool { return !spy.shutdownCalled }, time.Second, time.Millisecond)
	assert.Eventually(t, func() bool { return !spy.nextCalled }, time.Second, time.Millisecond)
}
