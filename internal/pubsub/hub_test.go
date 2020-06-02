// Copyright 2020 Canonical Ltd.

package pubsub_test

import (
	"testing"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/CanonicalLtd/jimm/internal/pubsub"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type hubSuite struct{}

var _ = gc.Suite(&hubSuite{})

func (s *hubSuite) TestSubscribeToModelMessages(c *gc.C) {
	hub := &pubsub.Hub{}

	messages := make(chan interface{}, 10)
	handlerFunc := func(model string, content interface{}) {
		select {
		case messages <- content:
		default:
			c.Fatalf("failed to send message")
		}
	}

	unsubscribe, err := hub.Subscribe("model1", handlerFunc)
	c.Assert(err, jc.ErrorIsNil)

	assertPublish(c, hub, "model1", "message1")
	assertMessage(c, messages, "message1")

	assertPublish(c, hub, "model2", "message2")
	assertMessage(c, messages, "")

	unsubscribe()

	assertPublish(c, hub, "model1", "message3")
	assertMessage(c, messages, "")
}

func (s *hubSuite) TestSubscribeToModelMessagesAfterMessagesHaveBeenSent(c *gc.C) {
	hub := &pubsub.Hub{}

	messages := make(chan interface{}, 10)
	handlerFunc := func(model string, content interface{}) {
		select {
		case messages <- content:
		default:
			c.Fatalf("failed to send message")
		}
	}

	assertPublish(c, hub, "model1", "message1")
	assertPublish(c, hub, "model1", "message2")

	unsubscribe, err := hub.Subscribe("model1", handlerFunc)
	c.Assert(err, jc.ErrorIsNil)
	defer unsubscribe()

	assertMessage(c, messages, "message2")
}

func (s *hubSuite) TestSubscribeMatcher(c *gc.C) {
	hub := &pubsub.Hub{}

	messages := make(chan interface{}, 10)
	handlerFunc := func(model string, content interface{}) {
		select {
		case messages <- content:
		default:
			c.Fatalf("failed to send message")
		}
	}
	matcher := func(model string) bool {
		switch model {
		case "model1", "model3":
			return true
		default:
			return false
		}
	}

	assertPublish(c, hub, "model1", "message1")
	assertPublish(c, hub, "model2", "message2")
	assertPublish(c, hub, "model1", "message3")

	unsubscribe, err := hub.SubscribeMatch(matcher, handlerFunc)
	c.Assert(err, jc.ErrorIsNil)

	// when we subscribe, we expect to receive message3, which
	// was the last message to be published about model1 before
	// we subscribed
	assertMessage(c, messages, "message3")

	// model3 matches
	assertPublish(c, hub, "model3", "message4")
	assertMessage(c, messages, "message4")

	// model2 does not match
	assertPublish(c, hub, "model2", "message5")
	assertMessage(c, messages, "")

	unsubscribe()

	// we expect no further messages
	assertPublish(c, hub, "model1", "message6")
	assertPublish(c, hub, "model3", "message7")
	assertMessage(c, messages, "")

}

type messageHub interface {
	Publish(string, interface{}) <-chan struct{}
}

func assertPublish(c *gc.C, hub messageHub, model string, message interface{}) {
	done := hub.Publish(model, message)
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		c.Fatal("timed out")
	}
}

func assertMessage(c *gc.C, messages chan interface{}, expectedMessage string) {
	var message interface{}
	select {
	case message = <-messages:
		if expectedMessage != "" {
			c.Assert(message, jc.DeepEquals, expectedMessage)
		} else {
			c.Fatal("received unexpected message")
		}
	default:
		if expectedMessage != "" {
			c.Fatal("message not received")
		}
	}
}
