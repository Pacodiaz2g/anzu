package events

import (
	"log"
	"time"

	"github.com/tryanzu/core/deps"
	"gopkg.in/mgo.v2/bson"
)

type Handler func(Event) error

// Input channel for incoming events.
var In chan Event

// On "event" channel. Register event handlers using channels.
var On chan EventHandler

// Map of handlers that will react to events.
var Handlers map[string][]Handler

type EventHandler struct {
	On      string
	Handler Handler
}

type Event struct {
	Name   string
	Sign   *UserSign
	Params map[string]interface{}
}

type eventLog struct {
	ID       bson.ObjectId          `bson:"_id,omitempty"`
	Name     string                 `bson:"name"`
	Sign     *UserSign              `bson:"sign,omitempty"`
	Params   map[string]interface{} `bson:"params,omitempty"`
	Created  time.Time              `bson:"created_at,omitempty"`
	Finished *time.Time             `bson:"finished_at,omitempty"`
}

type UserSign struct {
	Reason string
	UserID bson.ObjectId
}

func execHandlers(list []Handler, event Event) {
	defer func() {
		switch rval := recover().(type) {
		case nil:
			return
		case error:
			// Show the error
			log.Printf("[ERR] [events]: %+v\n", rval)

		default:
			// Show the error
			log.Printf("[ERR] [events]: %+v\n", rval)
		}
	}()
	starts := time.Now()
	ref := eventLog{
		ID:      bson.NewObjectId(),
		Name:    event.Name,
		Sign:    event.Sign,
		Params:  event.Params,
		Created: time.Now(),
	}
	err := deps.Container.Mgo().C("events").Insert(&ref)
	if err != nil {
		panic(err)
	}
	for h := range list {
		err = list[h](event)
		if err != nil {
			panic(err)
		}
	}
	finished := time.Now()
	elapsed := finished.Sub(starts)
	err = deps.Container.Mgo().C("events").UpdateId(ref.ID, bson.M{"$set": bson.M{
		"finished_at": finished,
		"elapsed":     elapsed,
		"handlers":    len(list),
	}})
	return
}

func sink(in chan Event, on chan EventHandler) {
	for {
		select {
		case event := <-in: // For incoming events spawn a goroutine running handlers.
			if ls, exists := Handlers[event.Name]; exists {
				go execHandlers(ls, event)
			} else {
				go execHandlers([]Handler{}, event)
			}
		case h := <-on: // Register new handlers.
			if _, exists := Handlers[h.On]; !exists {
				Handlers[h.On] = []Handler{}
			}

			Handlers[h.On] = append(Handlers[h.On], h.Handler)
		}
	}
}

// init channel for input events, consumers & map of handlers.
func init() {
	In = make(chan Event, 10)
	On = make(chan EventHandler)
	Handlers = make(map[string][]Handler)

	go sink(In, On)
}
