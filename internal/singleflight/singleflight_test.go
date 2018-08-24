// Copyright 2017 Canonical Ltd.

package singleflight_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gopkg.in/errgo.v1"

	"github.com/CanonicalLtd/jimm/internal/singleflight"
)

func TestContextCanceled(t *testing.T) {
	var g singleflight.Group
	ch := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	v, err := g.Do(ctx, "1", func() (interface{}, error) {
		cancel()
		<-ch
		return "test result", errors.New("test error")
	})
	if v != nil {
		t.Errorf("expected nil return value, but got %v", v)
	}
	if errgo.Cause(err) != context.Canceled {
		t.Errorf("expected %v, but got %v", context.Canceled, err)
	}
	close(ch)
	// give the background go-routine time to complete.
	time.Sleep(100 * time.Millisecond)
	v, err = g.Do(context.Background(), "1", func() (interface{}, error) {
		return "test result 2", errors.New("test error 2")
	})
	if s, ok := v.(string); !ok || s != "test result 2" {
		t.Errorf("expected %q return value, but got %#v", "test result 2", v)
	}
	if err == nil || err.Error() != "test error 2" {
		t.Errorf("expected %v, but got %v", errors.New("test error"), err)
	}
}

func TestSuppressDuplicates(t *testing.T) {
	var g singleflight.Group
	var count int32
	ch := make(chan string)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			v, err := g.Do(context.Background(), "test", func() (interface{}, error) {
				atomic.AddInt32(&count, 1)
				return <-ch, nil
			})
			if s, ok := v.(string); !ok || s != "test passed" {
				t.Errorf("expected %q return value, but got %#v", "test passed", v)
			}
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			wg.Done()
		}()
	}
	// give the go-routines time to block.
	time.Sleep(100 * time.Millisecond)
	ch <- "test passed"
	wg.Wait()
	if n := atomic.LoadInt32(&count); n != 1 {
		t.Errorf("expected the function to be run only once but was actually run %d times", n)
	}
}
